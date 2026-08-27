# ATLAS — Piano di implementazione

**Riferimento architetturale:** ANALYSIS.md (vincolante — in caso di dubbio, ANALYSIS.md decide)
**Linguaggio:** Go (binario statico) · **Git:** subprocess del `git` di sistema
**Metodo:** TDD rigoroso (test prima del codice, red → green → refactor) · **Coverage minima: 70%** (totale, `go tool cover -func`)
**Branching:** tutto su `main`, commit convenzionali per fase, NESSUNA attribuzione Claude nei commit.
**Scope:** oltre l'MVP — tutte le funzionalità di Fase 1 di ANALYSIS.md §15 più doctor completo, log query, policy, claims.

---

## S0. Struttura del progetto

```
go.mod                        # module github.com/dmarcocci/atlas
cmd/atlas/main.go             # entrypoint, exit code handling
internal/ledger/              # id, frontmatter codec, workitem, card, focus, log.jsonl, config
internal/gitx/                # wrapper subprocess git
internal/claims/              # claim-per-file, O_EXCL, TTL, steal
internal/state/               # stato derivato: ready, freshness
internal/contextc/            # compilatore del brief (testo + JSON, budget)
internal/doctor/              # verifiche di integrità
internal/cli/                 # comandi cobra, wiring, policy, bootstrap, seed
```

Dipendenze ammesse: `spf13/cobra`, `gopkg.in/yaml.v3`, `github.com/BurntSushi/toml`, `github.com/stretchr/testify`. Nient'altro.

Portabilità: `path/filepath` ovunque; creazione esclusiva con `os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)` (portabile: POSIX O_EXCL / Windows CREATE_NEW).

## S1. Modello dati su disco (`.atlas/`)

```
.atlas/
  focus.md            # markdown puro, 3-10 righe, nessun frontmatter
  work/<id>-<slug>.md # un workitem attivo per file
  cards/<id>-<slug>.md
  log.jsonl           # append-only, elementi chiusi
  config.toml         # opzionale, default sensati se assente
```

**ID:** 4 caratteri hex lowercase, generati random, ricontrollati contro collisioni con i file esistenti (work/, cards/ e log.jsonl); alla collisione rigenera (max 20 tentativi, poi 5 caratteri).
**Slug:** dal titolo, lowercase, `[a-z0-9-]`, max 40 char.

**Workitem** (frontmatter YAML + corpo markdown):
```markdown
---
id: a1b2
title: Fix container reconcile retry
status: todo            # todo | doing | blocked | done
created: 2026-08-27
blocked_by: [c3d4]      # opzionale
discovered_from: e5f6   # opzionale
branch: feature/retry   # impostato da `task start`
evidence:               # opzionale, lista di path (con :righe opzionali)
  - packages/core/pipeline/reconcile.py:120-180
summary: ""             # obbligatoria a done, 1 riga
reason: ""              # opzionale, motivo del blocco
---
Corpo markdown libero (intento, note, spec).
```

**Card:**
```markdown
---
id: k9m2
type: decision          # decision | knowledge
title: Usare O_EXCL per i claim
status: active          # active | superseded
superseded_by: ""       # id, per le decision superate
hook: "Claim = file O_EXCL in $GIT_COMMON_DIR, mai mutex"  # 1 riga per l'indice, OBBLIGATORIO
created: 2026-08-27
evidence: []
---
Corpo (per le decision: contesto/decisione/conseguenze, compatibile MADR).
```

**log.jsonl** (una riga per elemento chiuso):
```json
{"id":"a1b2","kind":"task","title":"...","summary":"...","closed":"2026-08-27T10:00:00Z","commit":"<HEAD short>","branch":"feature/retry"}
```
Per le card superate: `"kind":"card","superseded_by":"x1y2"`.

**config.toml** (tutti opzionali, questi i default):
```toml
[context]
budget_tokens = 1500
recent_days = 7

[policy]
plan_mutations = "warn"        # "warn" | "strict"
integration_branches = ["main", "develop"]

[claims]
ttl_hours = 24
```

**Claims** (fuori dal repo versionato): `<git-common-dir>/atlas/claims/<id>.json`
```json
{"id":"a1b2","branch":"feature/retry","session":"<ATLAS_SESSION o hostname-pid>","created":"2026-08-27T10:00:00Z","ttl_hours":24}
```
Acquisizione = creazione esclusiva atomica. Claim scaduto (created+ttl < now) = trattato come inesistente (sovrascrivibile con remove+retry create). Rilascio = delete a `done`. `--steal` = delete + create con warning su stderr.

## S2. Semantica dei comandi

Convenzioni globali: exit 0 = ok · exit 1 = errore I/O/parse · exit 2 = rifiuto semantico (claimed, policy strict, summary mancante, id inesistente) · exit 3 = solo `doctor` con problemi. Ogni comando di lettura e ogni rifiuto supportano `--json`. Errori JSON: `{"error":"<code>", ...campi utili...}`. Tutti i comandi localizzano la root del repo (dir con `.atlas/`, risalendo) tranne `init`.

- `atlas init` — crea `.atlas/focus.md` (template commentato) e `config.toml` coi default; aggiunge `.atlas/log.jsonl merge=union` a `.gitattributes`; installa il blocco bootstrap (S3) in AGENTS.md (append o creazione) e, se esiste CLAUDE.md, anche lì. Idempotente: blocchi delimitati `<!-- atlas:begin -->/<!-- atlas:end -->` sostituiti in place al re-run. Non tocca mai contenuto fuori dai marker.
- `atlas seed` — stampa su stdout il brief di curation (S4). Nessuna chiamata LLM. `--json` = `{"brief":"..."}`.
- `atlas context [id] [--json]` — compila il brief (S5). Con `id`: brief centrato (corpo integrale del workitem + card collegate via evidence/menzione id + suoi path). Include sempre freshness e claims attivi di altri branch ("in corso altrove").
- `atlas state [--json]` — vista completa leggibile: focus, tutti i workitem per stato, card attive con hook, ground git, freshness. Senza budget.
- `atlas task add "titolo" [--body -|"testo"] [--blocked-by id,id] [--from id] [--evidence p1,p2]` — crea workitem `todo`. Soggetto a policy plan-mutation SOLO se senza `--from` (il lavoro scoperto è sempre permesso).
- `atlas task start <id> [--steal]` — verifica claim (S1); crea claim; imposta `status: doing`, `branch: <corrente>`. Se già claimed da altro branch/sessione attiva: exit 2 con `{"error":"claimed","task":"..","by":"<branch>","ready":[...]}`. Se il workitem ha `branch` di un altro branch e status doing (binding versionato, macchine diverse): stesso rifiuto salvo `--steal`.
- `atlas task block <id> [--on id] [--reason "..."]` — `status: blocked`, aggiorna blocked_by/reason. Permesso solo dal branch che lo possiede (o se non ancora posseduto).
- `atlas task done <id> --summary "..."` — summary OBBLIGATORIA non vuota (exit 2 altrimenti); appende a log.jsonl (con HEAD short e branch), rimuove il file da work/, rilascia il claim. Permesso solo dal branch proprietario (o senza proprietario).
- `atlas card add --type decision|knowledge "titolo" [--hook "..."] [--body ...] [--evidence ...]` — hook obbligatorio (se assente usa il titolo). Soggetto a policy plan-mutation.
- `atlas card supersede <vecchio> <nuovo>` — vecchio→`status: superseded`, `superseded_by: nuovo`; appende evento a log.jsonl; il file superseded resta in cards/ ma escluso dal contesto. Policy plan-mutation.
- `atlas show <id> [--json]` — stampa il file integrale (JSON: frontmatter strutturato + body).
- `atlas log [--grep pattern] [--json]` — interroga log.jsonl (mai nel contesto).
- `atlas doctor [--json]` — verifica: blocked_by/discovered_from/superseded_by orfani; cicli in blocked_by; done senza summary nel log; focus non modificato da >N commit recenti (freshness, S5.2); claim scaduti o riferiti a workitem inesistenti (li rimuove con nota); card active più vecchie di 90 giorni mai toccate (warning); frontmatter malformati (parsing tollerante: segnala, non crasha). Exit 3 se problemi.

**Policy plan-mutation:** se il branch corrente non è in `integration_branches`: `warn` (default) → messaggio su stderr, procede; `strict` → exit 2 `{"error":"policy","branch":"..."}`. Mai applicata a: task con `--from`, transizioni start/block/done, comandi read-only.

## S3. Blocco bootstrap (installato da init)

```markdown
<!-- atlas:begin -->
## ATLAS
- A inizio sessione esegui `atlas context`: l'output è lo stato corrente del progetto.
- Prima di lavorare su un task: `atlas task start <id>` (se rifiutato: scegli un task dalla lista ready).
- Quando finisci un task: `atlas task done <id> --summary "una riga su cosa è cambiato"`.
- Decisione non ovvia presa? `atlas card add --type decision "titolo" --hook "sintesi di una riga"`.
- Lavoro nuovo scoperto? `atlas task add "titolo" --from <id-task-corrente>`.
- Prima di chiudere la sessione: aggiorna gli stati e, se il goal è cambiato, `.atlas/focus.md`.
- Usa `--json` sui comandi di lettura. Non modificare i file in `.atlas/` a mano: usa la CLI.
<!-- atlas:end -->
```

## S4. Brief di seed (testo costante stampato da `atlas seed`)

Contenuto (in inglese, per gli agenti): istruzioni per inventariare TODO/docs/ADR/git log recente e triagiare nel modello ATLAS con le regole lossy di ANALYSIS.md §12.2: focus 5-10 righe su oggi; max ~15 workitem SOLO aperti e rilevanti; card solo per decisioni ancora vincolanti, ADR referenziati via evidence MAI copiati; history esclusa (al più 1 card "lessons" + pointer); tutto via comandi CLI; lavorare su branch dedicato; chiudere con `atlas doctor`; l'umano rivede e committa. Includere gli esempi di comando.

## S5. Compilatore di contesto

**S5.1 Formato testo** (sezioni in quest'ordine, omesse se vuote):
```
# ATLAS CONTEXT (<data>) [STALE: ledger older than last N commits]   ← tag solo se stantio
## FOCUS
<focus.md verbatim>
## NOW
- [a1b2] titolo (doing, branch feature/x) — evidence: p1, p2
- [c3d4] titolo (blocked on e5f6: reason)
## READY
- [f6a7] titolo
## RULES
- [k9m2] hook (decision)
## RECENT
- [b2c3] summary (2026-08-25)
- git: <ultimi 5 commit oneline>
## GROUND
branch: feature/x · HEAD: abc1234 · worktree: dirty(3 files) · elsewhere: [a1b2 su feature/y]
## POINTERS
Dettaglio: `atlas show <id>` · Stato completo: `atlas state` · Storia: `atlas log --grep <x>`
```
**S5.2 Freshness:** stantio se mtime più recente tra i file di `.atlas/` è anteriore al timestamp del N-esimo commit più recente (N=5) E la working tree ha commit successivi. Esposta in GROUND e come tag in testata.
**S5.3 Budget:** stima token = len(runes)/4. Se oltre `budget_tokens`, degrada in ordine inverso di priorità (priorità: FOCUS > NOW > GROUND > READY > RULES > RECENT > POINTERS): prima RECENT ridotto a 3 righe poi rimosso, poi RULES ridotte a `[id] primi 60 char`, poi READY troncata con `… (+K altri: atlas state)`. FOCUS e NOW mai rimossi.
**S5.4 JSON:** `{"generated":ts,"stale":bool,"focus":"...","now":[{workitem}...],"ready":[...],"rules":[{"id","hook","type"}],"recent":[...],"ground":{"branch","head","dirty","elsewhere":[...]},"budget":{"limit":1500,"estimated":<n>}}`.
**S5.5 `context <id>`:** FOCUS + il workitem integrale (frontmatter+body) + card i cui id compaiono nel body/evidence del task o il cui evidence interseca i path del task + GROUND + POINTERS. Stesso budget.

## S6. gitx (subprocess)

Funzioni: `Root(dir)`, `CommonDir(dir)` (`git rev-parse --git-common-dir`, path assoluto), `Branch(dir)`, `HeadShort(dir)`, `IsDirty(dir)` (+conteggio file), `RecentCommits(dir,n)` (oneline), `CommitTimestamps(dir,n)`. Ogni funzione: esegue `git -C dir ...`, error wrapping con stderr. Se non in un repo git: le feature git degradano (context senza GROUND git, freshness non calcolabile) — MAI crash.

## S7. Strategia di test (TDD, vincolante)

- Ordine obbligatorio per ogni unità: scrivere i test (rossi) → implementare → verde → refactor. I commit di fase devono contenere test + implementazione.
- Helper condiviso `internal/testutil`: `SetupRepo(t)` → t.TempDir + `git init -b main` + config user locale + commit iniziale; `SetupWorktree(t, repo, branch)`. I test git usano SOLO repo temporanei; nessuna rete; deterministici (clock iniettabile ove serve: `Now func() time.Time` nei costruttori di claims/freshness).
- Integrazione CLI: test che invocano i comandi cobra via `Execute` con args e catturano stdout/stderr/exit (non serve compilare il binario nei test).
- Golden test per il rendering del contesto (testo e JSON).
- Casi obbligatori: collisione ID; frontmatter malformato (tolleranza); claim concorrente (2 goroutine che fanno O_EXCL sullo stesso id → esattamente una vince); claim scaduto riacquisibile; steal; done senza summary; policy warn vs strict; budget degradation (fixture oltre budget); init idempotente (doppio run → un solo blocco); merge=union in .gitattributes; ready con blocked_by chiusi nel log; cicli blocked_by (doctor); repo senza git.
- Coverage: `go test ./... -coverprofile=cover.out && go tool cover -func=cover.out | tail -1` ≥ 70% totale. `go vet ./...` pulito.

## S8. Fasi di esecuzione (una per agente, sequenziali)

1. **F1 — scaffold + ledger:** verifica toolchain (go, git; installare go via brew se assente), `git init` (main), go.mod, .gitignore (atlas binario, cover.out), testutil, internal/ledger completo (S1) in TDD. Commit `feat(ledger): core data model with TDD`.
2. **F2 — gitx + claims:** S6 e claims (S1) in TDD, incl. test worktree e concorrenza. Commit `feat(gitx,claims): git wrapper and atomic claims`.
3. **F3 — state + contextc:** ready/freshness (S5.2) e compilatore (S5) in TDD con golden test. Commit `feat(context): state derivation and budgeted context compiler`.
4. **F4 — CLI completa:** tutti i comandi (S2), bootstrap (S3), seed (S4), policy, in TDD con test d'integrazione. Commit `feat(cli): full command surface`.
5. **F5 — doctor + hardening:** doctor completo, riempimento gap di coverage fino a ≥70%, `go vet`, README.md (installazione, comandi, formato file), smoke test end-to-end in un repo temporaneo (init→seed→task add/start/done→context→doctor). Commit `feat(doctor): integrity checks; docs and coverage hardening`.

Regole per ogni fase: non modificare file di fasi precedenti se non necessario; suite SEMPRE verde a fine fase (`go test ./...`); riportare coverage di fase.
