# ATLAS — Analisi di Discovery e Architettura

**Ruolo:** Principal Software Architect (fase discovery — nessuna implementazione)
**Data:** 2026-08-27
**Stato:** bozza per discussione — nulla in questo documento è deciso definitivamente

---

## 1. Executive summary

Il problema che ATLAS deve risolvere **non è** la mancanza di documentazione, di specifiche o di memoria per gli agenti. L'ecosistema ne è saturo. Il problema è che **nessuno strumento esistente mantiene una rappresentazione compatta, affidabile e a basso costo di contesto dello stato corrente di un progetto**, tale che un agente di coding possa riprendere il lavoro senza ricostruire la storia.

Formulazione del problema:

> A ogni nuova sessione, un agente paga un "tasso di ricostruzione": deve leggere TODO storici, spec accumulate, ADR, log e codice per dedurre *dove siamo* e *cosa viene dopo*. Questo costo cresce con l'età del progetto, non con la dimensione del lavoro da fare.

La tesi di ATLAS: separare **stato corrente** (piccolo, curato, autorevole per l'intento) da **storia** (append-only, mai caricata di default), e offrire un comando che compili un **brief minimo sufficiente** fatto principalmente di *puntatori*, non di contenuti.

Tre conclusioni chiave della ricerca, anticipate:

1. **Il divario esiste davvero.** Spec Kit e OpenSpec gestiscono il *cambiamento* (feature → artefatti → archivio), non lo *stato corrente del lavoro*. I sistemi di memoria (Ruflo/claude-flow, Mem0, Zep) accumulano contesto invece di ridurlo. Le convenzioni (CLAUDE.md/AGENTS.md) coprono le *regole stabili*, non lo stato dinamico.
2. **Il concorrente più vicino non è uno strumento SDD ma Beads** (issue tracker git-nativo per agenti, di Steve Yegge): risolve già task-graph, ready-work detection e compattazione dei task chiusi. ATLAS si differenzia sul livello che Beads non ha — stato di progetto + decisioni + compilazione del contesto. **Decisione di prodotto (2026-08-27): ATLAS è standalone; nessun wrapper né dipendenza da Beads.** Beads resta prior art da cui riusare lezioni di design (id hash, ready-detection, compattazione alla chiusura).
3. **Il rischio principale non è tecnico ma comportamentale:** qualsiasi stato curato diventa stantio se agenti e umani non lo aggiornano. L'architettura deve rendere l'aggiornamento più economico della non-manutenzione, e deve saper *rilevare* la staleness invece di fingere che non esista.

---

## 2. Analisi dei pain point

### A. Dolore del developer (validato concettualmente)

| Pain point | Reale? | Vale la pena risolverlo in ATLAS? |
|---|---|---|
| Distinguere stato corrente da storia | **Sì, è il pain centrale.** Il TODO.md da 2.000 righe è il sintomo canonico: un log travestito da stato. | **Sì — è il cuore del prodotto.** |
| Ricordare il perché delle decisioni | Sì, ma è già ben servito dagli ADR *come formato*; il fallimento è di processo (nessuno li legge/aggiorna). | Parzialmente: ATLAS deve *indicizzare* le decisioni nel contesto, non reinventare l'ADR. |
| TODO stantii / task drift | Sì. Deriva dall'assenza di un ciclo di vita: i task non hanno uno stato "chiuso e compattato". | Sì, con lifecycle esplicito e compattazione. |
| Spec in conflitto / duplicate | Sì nei sistemi SDD (verificato: issue OpenSpec #678, #1387). Causato dal modello "spec per feature" senza livello canonico. | Indirettamente: ATLAS non deve avere spec-per-feature accumulate. |
| ADR superati e fuorvianti | Sì (letteratura ADR: "ADR del 2021 letto alla lettera è attivamente fuorviante"). | Sì, ma con un meccanismo leggero: stato `superseded` + esclusione dal contesto di default. |
| Trovare il lavoro corrente | Sì. Beads lo chiama problema "50 First Dates". | Sì. |
| Documentazione duplicata | Sì, ma è un problema generale di igiene documentale; ATLAS non può risolverlo per tutto il repo. | No come obiettivo diretto; sì come conseguenza (una sola sede per lo stato). |

### B. Dolore dell'agente (cause di consumo eccessivo di contesto)

Ordinate per impatto stimato:

1. **Ricostruzione dello stato da log storici** — leggere 2.000 righe per estrarne 50 rilevanti. Rapporto segnale/rumore pessimo; peggiora con l'età del progetto.
2. **Context rot** — verificato (ricerca Chroma, 18 modelli frontier): le prestazioni degradano in modo non uniforme e a scalino al crescere dei token, anche su task banali. Ogni token irrilevante non è solo un costo economico: riduce l'affidabilità.
3. **Retrieval troppo ampio** — i sistemi di memoria (Ruflo: AgentDB con vettori+grafo, ~210 tool MCP) iniettano più di quanto serva; la stessa superficie di tool consuma contesto. Conferma l'intuizione iniziale: più memoria ≠ meglio.
4. **Riletture ripetute** — l'agente riscopre a ogni sessione la struttura del repo e le convenzioni. In parte già mitigato da CLAUDE.md/AGENTS.md (regole stabili), non dallo stato dinamico.
5. **Discontinuità di sessione** — la compaction automatica perde istruzioni iniziali; la prassi comunitaria emergente è il "handoff brief" manuale a fine sessione. ATLAS può standardizzare esattamente questo.
6. **Informazione duplicata/stantia** — spec che ripetono il codice, TODO che contraddicono lo stato reale. Peggio dell'assenza: l'agente non sa a chi credere.

**Osservazione architetturale:** i punti 1, 5, 6 sono risolvibili con un *ledger di stato piccolo e curato*; i punti 2, 3 impongono che il contesto compilato sia fatto di **puntatori + sintesi minime**, mai di dump; il punto 4 è già risolto dall'ecosistema e ATLAS non deve duplicarlo.

### C. Ciclo di vita dell'informazione (sintesi)

| Tipo | Chi la crea | Quando diventa stantia | Autorevole? | Nel contesto di default? | Provenienza necessaria? |
|---|---|---|---|---|---|
| Codice | dev + agente | mai (è la verità) | **Sì — fatto** | No (letto just-in-time) | no (è la fonte) |
| Git history | automatica | mai (append-only) | Sì — per gli eventi | No (interrogata on demand) | no |
| Task attivi | dev/agente | in giorni | Sì — per l'intento | **Sì** | utile (link a spec/commit) |
| Task chiusi | transizione di stato | subito dopo la chiusura | solo storicamente | **No** (solo sintesi di 1 riga se recente) | sì (commit che chiude) |
| Decisioni attive | dev (spesso con agente) | quando superate | Sì — per i vincoli | Sì, in forma di indice | sì (alternativa scartata, data) |
| Decisioni superate | transizione di stato | — | no | No | — |
| Spec / intento di feature | dev | quando implementate | Sì finché attive | Solo se legate al task corrente | utile |
| Conoscenza del progetto (gotcha, mappa, convenzioni) | dev/agente | lentamente | media | Indice sì, corpo on demand | consigliata (path/commit) |
| Log di lavoro agente | agente | immediatamente | no | **Mai** | — |

### D. Failure mode (anticipazione — dettaglio in §13)

I quattro più pericolosi: **stato stantio creduto vero**, **doppia verità** (stato vs codice/git in disaccordo), **write-back mancante** (l'agente lavora ma non aggiorna ATLAS), **conflitti concorrenti** (due agenti/branch modificano lo stato). L'architettura è progettata attorno a questi quattro.

---

## 3. Analisi dell'ecosistema esistente

Sintesi della ricerca (fonti primarie verificate ad agosto 2026; citazioni nei rapporti di ricerca).

### GitHub Spec Kit (~132k stelle, attivo)
- **Modello:** `.specify/` (template, constitution) + `specs/<branch>/` con spec.md, plan.md, tasks.md, research.md, data-model.md, contracts/. Workflow a slash command: constitution → specify → plan → tasks → implement.
- **Cosa fa bene:** distribuzione multi-agente via file di slash command nativi (30+ agenti); il concetto di *constitution* (vincoli stabili di progetto).
- **Cosa fa male (verificato):** ceremonia estrema — recensione Scott Logic: 2.577 righe di markdown per 689 righe di codice, 3,5 ore di revisione di documenti vs ~8 minuti con prompting incrementale. Nessun livello di "spec corrente del sistema": le dir per feature si accumulano e derivano. `/speckit.converge` è una toppa successiva per il brownfield.
- **Lezione per ATLAS:** la distribuzione file-based agent-nativa è la convenzione vincente; il modello "artefatti per feature senza stato canonico" è l'anti-pattern da evitare.

### OpenSpec (~66k stelle, attivo)
- **Modello:** due livelli espliciti — `openspec/specs/` = verità corrente, `openspec/changes/<nome>/` = delta in corso (ADDED/MODIFIED/REMOVED), `changes/archive/` = storia. Il passo di archive fonde i delta nella verità corrente.
- **Cosa fa bene:** è l'unico ad aver formalizzato **corrente ≠ delta ≠ storia** — esattamente la distinzione che ATLAS vuole, applicata però alle sole specifiche.
- **Cosa fa male (verificato):** le spec sono advisory e derivano comunque (risync manuale); cambi paralleli sulla stessa requirement confliggono (issue #1387); resta un workflow a 4+ comandi per ogni cambiamento.
- **Lezione per ATLAS:** riusare il modello a due livelli, ma applicarlo allo *stato del progetto* e renderlo opzionale, non un rito per ogni modifica.

### ADR / MADR
- **Convenzione consolidata:** Nygard (Context/Decision/Status/Consequences) e MADR 4.0 (frontmatter YAML con status proposed/accepted/deprecated/superseded-by, file `NNNN-titolo.md` in `docs/decisions/`). Regola: mai cancellare, marcare superseded.
- **Failure mode noti:** "ADR come teatro" (scritti, mai letti), staleness fuorviante, catene di supersession rotte. La letteratura recente converge su: ADR come *log append-only* + documento di stato corrente separato + inserimento nel contesto degli agenti perché vengano effettivamente letti.
- **Lezione per ATLAS:** non inventare un nuovo formato di decisione. Adottare frontmatter compatibile MADR e risolvere il problema che MADR non risolve: far *arrivare* le decisioni attive nel contesto dell'agente.

### AGENTS.md / CLAUDE.md
- AGENTS.md: standard Linux Foundation (Agentic AI Foundation), 20-30+ agenti, markdown libero, regola nearest-file (con divergenze: Codex fa merge root→cwd). Claude Code non lo legge nativamente (serve import/symlink).
- CLAUDE.md: gerarchia managed/user/project/local, import `@file` (profondità 4), regole path-scoped in `.claude/rules/`, auto-memory con indice MEMORY.md (primi 200 righe/25KB caricati) + topic file on demand, linea guida "sotto 200 righe".
- **Lezione per ATLAS:** questi file sono il **punto d'aggancio**, non i concorrenti: contengono regole stabili e possono contenere le 5 righe che dicono all'agente "esegui `atlas context` a inizio sessione". Il pattern Claude Code "indice piccolo sempre caricato + dettaglio on demand" è la conferma di prodotto del modello di contesto di ATLAS.

### Ruflo (ex claude-flow, ~70k stelle)
- **Cos'è:** meta-harness di orchestrazione: AgentDB (SQLite + vettori HNSW), swarm gerarchici, ~210 tool MCP, 27 hook, consensus Raft/bizantino.
- **Criticità (verificate):** overkill dichiarato perfino dai suoi doc per il lavoro normale; benchmark auto-riportati non validati; l'iniezione always-on di memoria è l'esatto anti-obiettivo di ATLAS.
- **Lezione per ATLAS:** conferma per contrasto: niente DB, niente embeddings, niente daemon, niente MCP obbligatorio.

### Memoria generica (Mem0, Letta/MemGPT, Zep/Graphiti, Cognee)
Nessuno è pensato per coding agent; tutti convergono su "estrai fatti → store vettoriale/grafo → inietta nel prompt". Il concetto utile da Zep/Graphiti è la **bi-temporalità** (quando un fatto era vero vs quando è stato appreso) — troppo pesante da implementare, ma il principio "i fatti superati si invalidano, non si cancellano" va tenuto.

### Beads (Steve Yegge) — **il vicino più prossimo**
- Issue tracker git-nativo per agenti: issue come DAG (blocks, parent-child, related, **discovered-from**), ID hash anti-collisione (`bd-a1b2`), **`bd ready`** (lavoro sbloccato, output JSON), compattazione AI dei task chiusi ("memory decay"), `bd prime` (contesto di lavoro), protocollo di fine sessione ("land the plane"). Storage: JSONL in git.
- **Cosa NON copre:** stato di progetto oltre i task (fase, decisioni, conoscenza, spec), compilazione del contesto oltre la lista dei task pronti.
- **Verdetto onesto:** se ATLAS si riducesse a task + ready-detection, **sarebbe una reinvenzione di Beads e non varrebbe la pena costruirlo**. Il valore differenziale di ATLAS sta nel livello sopra i task: stato + decisioni + brief compilato. Vedi §14 per l'alternativa "adottare Beads".

### Cosa manca nell'ecosistema (il divario di ATLAS)
1. Nessuno strumento risponde in O(1) alla domanda: *"qual è lo stato corrente del progetto?"* — tutti rispondono con log, archivi o retrieval.
2. Nessuno compila un **brief minimo** cross-artefatto (stato + task + decisioni + puntatori) pensato per il budget di attenzione di un LLM.
3. Nessuno tratta la **staleness come proprietà misurabile** (stato vs git HEAD) invece che come speranza.

---

## 4. Confini del prodotto

### ATLAS È
- Un **ledger di stato di progetto**: piccolo, versionato in git, leggibile da umani, interrogabile da CLI.
- Un **compilatore di contesto**: da ledger + git produce un brief minimo sufficiente (testo + JSON).
- Un **protocollo di handoff** tra sessioni, agenti diversi e umani: chiunque riprende dal ledger, non dalla storia.
- **Agent-neutral e file-based**: funziona con qualunque agente capace di eseguire un comando o leggere un file.
- **Adottabile in modo incrementale**: `atlas init` su un repo esistente non richiede ristrutturazioni; convive con TODO.md, docs/, ADR esistenti (li può referenziare).

### ATLAS NON È
- **Non è un framework SDD**: nessun workflow obbligatorio spec→plan→tasks→implement; le spec sono opzionali e collegate ai task, non riti.
- **Non è un sistema di memoria/retrieval**: niente embeddings, vettori, knowledge graph, estrazione automatica di fatti.
- **Non è un orchestratore**: non lancia agenti, non fa swarm, non coordina.
- **Non è un issue tracker completo**: niente assegnatari, sprint, priorità elaborate, board (se serve quello, meglio Beads o un tracker vero).
- **Non è documentazione**: non sostituisce docs/, README, ADR; li indicizza.
- **Non richiede infrastruttura**: nessun server, DB, daemon, MCP obbligatorio, cloud.

---

## 5. Modello concettuale

Le 7 categorie iniziali (stato, conoscenza, spec, decisioni, task, storia, evidenza) **non devono diventare 7 entità**. Analisi:

- **STORIA** non è un'entità: è la *proprietà* di qualsiasi elemento non più attivo (+ git stesso). Non si modella, si archivia.
- **EVIDENZA** non è un'entità: è un *campo* (link a file/righe/commit/test) sugli elementi che ne hanno bisogno.
- **STATO CORRENTE** non è un'entità immagazzinata: è una **vista** = elementi attivi + fatti derivati da git. Immagazzinarlo come documento autonomo creerebbe la doppia verità (vedi §7).
- **SPEC** non merita un'entità di prima classe nell'MVP: l'intento di una feature vive come corpo esteso di un task (o file collegato). I sistemi spec-per-feature sono la fonte principale di deriva documentale (verificato su Spec Kit/OpenSpec). Riaperto in §16 se l'uso lo giustifica.
- **CONOSCENZA** e **DECISIONE** sono simili (entrambe "cose che l'agente deve sapere") ma con lifecycle diverso: la decisione ha supersession e rationale; la nota di conoscenza è un fatto pratico aggiornabile in place. Si possono unificare in un'unica entità con `type` diverso.

**Modello minimo proposto: 2 entità immagazzinate + 2 viste derivate.**

1. **WORKITEM** (task) — unità di lavoro con lifecycle. Campi: `id` (hash corto anti-collisione, stile Beads), `title`, `status` (todo | doing | blocked | done), `blocked_by` (lista id), `discovered_from` (id, opzionale), corpo markdown libero (può contenere l'intento/spec), `evidence` (opzionale), `summary` (1 riga, compilata alla chiusura).
2. **CARD** (conoscenza/decisione) — fatto durevole che l'agente deve conoscere. Campi: `id`, `type` (decision | knowledge), `title`, `status` (active | superseded), `superseded_by` (id, per le decision), corpo markdown (per le decision: formato compatibile MADR), `evidence` (opzionale), `hook` (1 riga per l'indice).
3. **STATE** *(vista, non file autorevole)* — proiezione: goal corrente + workitem attivi/bloccati + card attive (indice) + segnali git (branch, ultimi commit, diff non committato) + valutazione di freshness. Unico frammento *immagazzinato* dello stato: un file `focus` di poche righe (goal corrente, fase, prossima cosa) — l'unica parte che non è derivabile.
4. **CONTEXT** *(vista, non file)* — il brief compilato da STATE secondo il modello di §8.

**Sfida accolta rispetto all'idea iniziale:** "STATE.md" come documento mantenuto a mano è l'astrazione sbagliata — è esattamente il TODO.md da 2.000 righe che rinasce. Lo stato deve essere *quasi tutto derivato*; solo il "focus" (intento corrente) è dichiarato, perché l'intento non è derivabile da git.

---

## 6. Ciclo di vita dell'informazione

    creazione → attivo → superseded/done → compattato → storia

- **Creazione:** un comando (`atlas task add`, `atlas card add`) o un file scritto a mano nel formato giusto — le due vie devono essere equivalenti (il CLI è una comodità, non un gatekeeper).
- **Attivo:** l'elemento è visibile in STATE e candidato al contesto. I workitem `doing` e `blocked` pesano di più dei `todo`.
- **Chiusura/supersession:** transizione esplicita (`atlas task done X`, `atlas card supersede A B`). Alla chiusura il workitem DEVE ricevere una `summary` di 1 riga (fornita dall'agente/umano che chiude — è il momento in cui il contesto per scriverla esiste già, costo marginale ~zero). È la versione leggera del "memory decay" di Beads.
- **Compattazione:** gli elementi chiusi escono dai file attivi e finiscono in un log append-only (JSONL o markdown datato). Nel contesto di default sopravvive solo la `summary` dei chiusi *recenti* (ultimi N giorni), come ponte tra sessioni.
- **Storia:** il log + git. Mai caricata di default; interrogabile (`atlas log`, `git log`). Niente cancellazioni: come per gli ADR, la storia si invalida, non si riscrive.

Regola di igiene fondamentale: **ogni transizione di stato è anche un'operazione di riduzione del contesto futuro** (chiudere = comprimere). È l'inverso del TODO.md, dove ogni evento *aggiunge* righe.

---

## 7. Modello di stato

**Definizione:** lo STATO CORRENTE è l'insieme minimo di asserzioni vere *adesso* che non sono ricavabili economicamente dal codice o da git:

1. **Intento:** su cosa stiamo lavorando e perché (focus, poche righe).
2. **Lavoro:** workitem attivi, bloccati e pronti (derivato dal ledger).
3. **Vincoli:** decisioni attive che limitano le scelte (indice delle card).
4. **Terreno:** segnali dal repository — branch, ultimi commit, stato del worktree (derivato da git, mai immagazzinato).
5. **Affidabilità:** quanto è fresco tutto ciò (derivato: ultima modifica del ledger vs ultimi commit).

**Risposta alla domanda architetturale "cos'è STATE":** una **vista materializzata on demand** — mai un documento mantenuto a mano (deriva), mai un DB (infrastruttura ingiustificata), mai una sintesi scritta da un agente senza struttura (non verificabile). La parte dichiarata (focus) è deliberatamente così piccola che tenerla aggiornata costa meno di una riga di commit message.

**Fonte di verità (domanda §1 dell'input):** stratificata, senza ambiguità:
- **Fatti sul software** → codice + git. ATLAS non li duplica mai.
- **Intento e stato del lavoro** → ledger ATLAS.
- **Vincoli e rationale** → card ATLAS (o ADR esistenti referenziati).
- In caso di conflitto tra ledger e git, **git vince sui fatti, il ledger vince sull'intento**, e il conflitto stesso è un segnale che ATLAS deve esporre (freshness check), non nascondere.

---

## 8. Modello di contesto

**Definizione:** il MINIMO CONTESTO SUFFICIENTE è il più piccolo insieme di token che permette all'agente di (a) sapere cosa fare, (b) sapere cosa NON fare, (c) sapere **dove guardare** per tutto il resto.

Principio strutturale (allineato al consenso 2025-26 su context engineering: "smallest set of high-signal tokens", just-in-time retrieval, progressive disclosure):

> **Il contesto compilato contiene sintesi e puntatori, mai contenuti integrali.** L'agente moderno è bravissimo a leggere file just-in-time; il collo di bottiglia è sapere *quali* file e *quale* stato. ATLAS fornisce la mappa, non il territorio.

Struttura del brief (budget target: **< 1.500 token** nel caso tipico, hard cap configurabile):

    [FOCUS]      goal corrente, fase                          (~3 righe, dichiarato)
    [NOW]        workitem doing/blocked + motivo del blocco   (~1 riga ciascuno + path rilevanti)
    [READY]      workitem sbloccati, in ordine                (1 riga ciascuno)
    [RULES]      card attive: hook di 1 riga + id             (l'agente apre la card solo se serve)
    [RECENT]     summary dei chiusi recenti + ultimi commit   (ponte tra sessioni)
    [GROUND]     branch, worktree sporco/pulito, freshness    (~3 righe, derivato)
    [POINTERS]   come approfondire: atlas show <id>, path     (~3 righe)

Due formati dello stesso contenuto: **testo** (per iniezione nel prompt, leggibile da umani) e **JSON** (`--json`, per tooling). Il contesto è **parametrico**: `atlas context` = brief generale; `atlas context <id>` = brief centrato su un workitem (il suo corpo integrale + le sole card collegate + i suoi path).

**Sfida accolta:** "context = documento generato dal CLI" è giusto ma con un vincolo che l'idea iniziale non esplicitava — se il compilatore inizia a *includere* conoscenza invece di *puntarla*, ATLAS degenera nel sistema di retrieval che non vuole essere. Il budget di token è un vincolo architetturale di prima classe, non un'ottimizzazione.

---

## 9. Compilazione del contesto (algoritmo concettuale)

Nessuna semantica, nessun embedding: **selezione per stato e per collegamento espliciti**, più segnali git. Deterministico, spiegabile, testabile.

    input: ledger (workitem, card, focus), repo git, [id target opzionale]

    1. GROUND    ← branch corrente, HEAD, dirty/clean, ultimi K commit (git, read-only)
    2. FRESHNESS ← confronto timestamp ledger vs commit recenti;
                   se il ledger non è toccato da N commit/giorni → flag "stato possibilmente stantio"
    3. ACTIVE    ← workitem status ∈ {doing, blocked}
       READY     ← workitem todo senza blocked_by aperti (stile `bd ready`)
    4. RULES     ← card active; se c'è un target, prima le card linkate al target
    5. RECENT    ← summary dei done negli ultimi N giorni (default piccolo)
    6. TARGET    ← se richiesto un id: corpo integrale del workitem + evidence + card collegate
    7. BUDGET    ← rendering nell'ordine FOCUS > NOW > GROUND/FRESHNESS > READY > RULES > RECENT;
                   se si supera il budget si troncano le sezioni meno prioritarie
                   (mai FOCUS/NOW), degradando da sintesi a solo-id
    8. output    ← testo o JSON

Punti deliberatamente esclusi: retrieval semantico (non necessario con collegamenti espliciti e agenti capaci di grep), ranking probabilistico (opaco), lettura del codice sorgente (compito dell'agente, just-in-time). La selezione per rilevanza sui *file* è delegata ai path citati nei workitem — evidence esplicita, non inferenza.

---

## 10. Modello di repository

Il più piccolo layout ragionevole — file-based, git-versionato, leggibile e scrivibile a mano:

    .atlas/
      focus.md            # 3-10 righe: goal, fase, prossima cosa (unico stato dichiarato)
      work/
        <id>-<slug>.md    # un workitem attivo per file (frontmatter YAML + corpo markdown)
      cards/
        <id>-<slug>.md    # una card per file (frontmatter compatibile MADR per le decision)
      log.jsonl           # append-only: elementi chiusi compattati (id, title, summary, ts, commit)
      config.toml         # opzioni minime (budget, N giorni recenti) — opzionale, default sensati

Scelte e razionali:
- **Un file per elemento attivo** (non un file monolitico): merge git quasi senza conflitti, editabile a mano, diff leggibili. È la lezione di Beads (JSONL/file in git) e di MADR (un file per decisione).
- **Markdown + frontmatter YAML**: il formato che tutti gli agenti e gli umani già leggono/scrivono; la parte strutturata sta nel frontmatter, la prosa nel corpo. Niente formato binario, niente DB. (Valutato TOML/JSON puri: perdono la prosa; valutato SQLite: infrastruttura e opacità ingiustificate a questa scala — decine di elementi attivi, non migliaia.)
- **`log.jsonl` append-only** per la storia: economico, greppabile, mai in contesto.
- **Directory nascosta `.atlas/`**: segnala "gestito da uno strumento" ma resta ispezionabile; in git di default (la condivisione dello stato è il punto; il caso "stato personale non condiviso" è rimandato a §17).
- **Adozione incrementale**: `atlas init` crea solo `.atlas/focus.md`; tutto il resto nasce al primo uso. Nessun obbligo di migrare TODO/ADR esistenti — le card possono puntarli.

---

## 11. Modello CLI

Il CLI più piccolo coerente col modello — **~8 comandi**, due dei quali fanno il 90% del lavoro:

    atlas init                          # crea .atlas/ + installa il contratto in AGENTS.md/CLAUDE.md (blocco marcato, idempotente)
    atlas seed                          # brownfield: emette il brief di curation che l'agente esegue (§12.2)
    atlas context [id] [--json]         # IL comando: compila il brief
    atlas state                         # vista umana dello stato (superset leggibile di context)

    atlas task add "titolo" [--blocked-by id] [--from id]
    atlas task start|block|done <id> [--summary "..."]   # done richiede summary; start registra branch+claim (§12.1)
    atlas card add --type decision|knowledge "titolo"
    atlas card supersede <vecchio> <nuovo>
    atlas show <id>                     # corpo integrale di un elemento

    atlas doctor                        # freshness + integrità (id orfani, blocchi ciclici, focus vecchio)

Principi:
- **Ogni comando di lettura ha `--json`.** Gli agenti non devono parsare markdown.
- **Il CLI non è un gatekeeper:** editare i file a mano è supportato; `atlas doctor` verifica la coerenza invece di impedirla.
- **Niente sottocomandi di workflow** (no plan/approve/archive/sync): il lifecycle è tutto in `task done` e `card supersede`.
- Il nome breve dei comandi conta: l'agente li invoca in autonomia e ogni token del comando è contesto.

---

## 12. Integrazione con gli agenti

Meccanismo unico, tre livelli di aggancio — **nessun MCP richiesto**:

1. **Bootstrap (obbligatorio, ~5-10 righe), installato da `atlas init` — mai copia-incolla manuale.** Il contratto comportamentale:
   *"A inizio sessione: `atlas context` → è lo stato corrente. Prima di lavorare su un task: `atlas task start <id>`. Quando finisci: `atlas task done <id> --summary '...'`. Decisione non ovvia: `atlas card add`. Lavoro scoperto: `atlas task add --from <id>`. Prima di chiudere la sessione: aggiorna stati e focus."*
   `atlas init` lo scrive come **blocco marcato idempotente** (`<!-- atlas:begin --> … <!-- atlas:end -->`) nei file agente rilevati: append ad AGENTS.md se esiste; CLAUDE.md per Claude Code (o import `@AGENTS.md`); creazione di AGENTS.md se non esiste nulla. Rilanciare `init` aggiorna solo il blocco, mai il resto del file (stesso pattern di `openspec update`). Funziona identico per Claude Code, Codex, Cursor, OpenCode: tutti leggono il proprio file di istruzioni ed eseguono comandi shell — il minimo comune denominatore reale dell'ecosistema (verificato: è lo stesso canale usato da Spec Kit/OpenSpec/Beads).
2. **Comodità per-agente (opzionale):** slash command / skill sottili (`/atlas-context`, `/atlas-done`) generati da `atlas init --integration <agente>`, sul modello Spec Kit. Solo wrapper del CLI, mai logica.
3. **Fine sessione (protocollo, non software):** il bootstrap include l'equivalente del "land the plane" di Beads: prima di chiudere, aggiorna gli stati e il focus. È qui che si gioca il write-back problem (§13).

Principio trasversale — **il contratto contiene il *quando*, mai il *come*.** Tutta la logica (atomicità, policy, budget, freshness) vive nel binario; le istruzioni situazionali vivono negli **output del CLI, just-in-time**: un `task start` rifiutato risponde "claimed da feature/a — alternative ready: cd34, ef56"; `atlas context` chiude col promemoria di write-back. Ogni token così speso arriva solo quando è rilevante, invece di pesare su ogni sessione. Metrica di guardia: se servissero 50 righe di istruzioni statiche per usare ATLAS, sarebbe un difetto di design del CLI, non un problema di documentazione.

Gli umani usano gli stessi comandi o editano i file. Non c'è un percorso separato "per agenti".

---

## 12.1 Concorrenza: worktree, branch e agenti paralleli

Scenario reale (emerso in review): più agenti lavorano in parallelo su worktree/branch diversi dello stesso repo. Con il ledger versionato in git, ogni worktree vede la *propria* copia di `.atlas/`: nascono due problemi distinti, da trattare separatamente.

1. **Race di visibilità** — l'agente B non vede che A (in un altro worktree) ha preso in carico o chiuso un task finché non avviene un merge: READY stantio, doppia presa in carico dello stesso lavoro.
2. **Conflitti di merge** — due branch modificano gli stessi file di `.atlas/`.

Valutazione delle due proposte sul tavolo:

- **"Esternalizzare il controller fuori dal repository"** — come store primario: no. Si perderebbero versionamento, condivisione col team via git, review dei cambi di stato nelle PR e il principio repository-local (§4). Ma esiste una via di mezzo tecnica: tutti i worktree di un repo condividono la directory git comune (`git rev-parse --git-common-dir`). Un **layer di coordinamento effimero** può vivere lì (`.git/atlas/claims/<task-id>.json` — un file per claim): condiviso istantaneamente tra tutti i worktree della macchina, invisibile a git, senza problemi di merge. L'atomicità è quella del filesystem: acquisire un claim = creazione esclusiva atomica del file — `O_CREAT|O_EXCL` su POSIX, `CreateFile`+`CREATE_NEW` su Windows, esposta in modo portabile da tutti i runtime (Go `O_EXCL`, Rust `create_new`, Node `'wx'`, Python `'x'`; è la stessa primitiva su cui git basa `index.lock` su ogni piattaforma). Una singola operazione atomica: il secondo che ci prova riceve `EEXIST`/`ERROR_FILE_EXISTS`, che è direttamente l'esito semantico "già preso" — **niente mutex, niente logica di retry da nessuna parte, né nel CLI né nell'agente**: si elimina la risorsa condivisa mutabile invece di proteggerla. È una **cache, non verità**: ricostruibile in ogni momento dal ledger; se manca, ATLAS degrada con grazia.
- **"CLI solo su develop"** — troppo forte se applicato a tutto: ucciderebbe il write-back nel momento in cui il contesto esiste (il `task done --summary` avviene nel worktree della feature; rimandarlo a develop reintroduce esattamente il problema che vogliamo risolvere). Giusto invece se si separa **per classe di operazione** (lo stesso pattern "plan mutations solo da develop" già adottato da tool come aiops-ai-spec).

**Modello proposto — proprietà per binding, coordinamento effimero, piano centralizzato:**

1. **Binding task↔branch:** `atlas task start` registra il branch corrente nel frontmatter (`branch:`). Da quel momento solo quel branch modifica quel file: il CLI rifiuta (o avverte, secondo policy) le modifiche a un workitem preso in carico altrove. Con un-file-per-elemento, i conflitti di merge diventano strutturalmente rari: ogni branch tocca solo i "suoi" file.
2. **Classi di operazione:**
   - *Read-only* (`context`, `state`, `show`, `doctor`): sempre consentite, in qualunque worktree — nessun rischio.
   - *Transizioni del task assegnato al branch* (`start/block/done`, più `task add --from <id>` per il lavoro scoperto durante l'implementazione): consentite nel worktree della feature.
   - *Mutazioni di piano* (task nuovi scollegati, `card add/supersede`, modifica del focus): raccomandate solo sul branch di integrazione (develop/main). Policy configurabile: `warn` di default (i repo single-dev senza gitflow esistono), `strict` per i team.
3. **Claims effimeri in `$GIT_COMMON_DIR`:** a `task start` il claim (branch, sessione, timestamp con TTL nel contenuto) viene creato nel layer condiviso; `atlas context` da qualunque worktree lo legge e mostra "in corso altrove". Un `task start` su un task già claimed **fallisce fail-fast con esito semantico** — mai attesa, mai coda: in `--json` restituisce il motivo e le alternative READY (`{"error":"claimed","by":"feature/a","ready":["cd34","ef56"]}`), e l'agente passa ad altro. La valvola per il caso legittimo ("B deve proprio lavorare su quel task", o A è morto prima della scadenza TTL) è `--steal`: esplicita, rumorosa, mai un fallback automatico. Rilascio = cancellazione del file a `done`; i claim orfani scadono per TTL. Risolve la race di visibilità sulla stessa macchina senza server, senza daemon e senza lock bloccanti.
4. **Limite dichiarato — macchine diverse:** i claim non attraversano le macchine. Lì restano il binding advisory nel frontmatter (versionato, quindi visibile dopo push/pull) e il merge git. Risolverlo davvero richiederebbe un server centrale: fuori scope per principio (§4).
5. **Merge del log:** `log.jsonl` marcato `merge=union` in `.gitattributes` — gli append da branch diversi si fondono senza conflitto.

Impatto sul resto del documento: §11 (il CLI acquisisce policy e claim implicito in `task start`), §13.3 (riscritto di conseguenza), §15 (il binding entra nell'MVP; il layer claims può slittare alla fase successiva), §17 (nuova questione aperta n.9).

---

## 12.2 Seeding di repository esistenti (brownfield)

Il caso d'adozione primario non è il progetto greenfield ma il repo maturo: centinaia di file markdown, ADR, TODO monolitici con anni di storia (caso reale di riferimento: ~24k righe di markdown in 284 file, ADR esistenti, un TODO/history enorme). Il triage manuale è impraticabile; serve assistenza LLM. La domanda architetturale è *dove* vive l'LLM.

**Principio: il binario resta LLM-free.** Chiamate a modelli dentro `atlas` significherebbero API key, dipendenza da un vendor, costi e non-determinismo in un tool deterministico — violazione diretta di agent-neutrality (§4) e "nessuna infrastruttura". L'LLM per il seed c'è già: **è il coding agent dell'utente**.

**Meccanica — `atlas seed` emette, l'agente esegue:**

1. `atlas seed` non chiama nessun modello: **stampa il brief di curation** (applicazione del principio "istruzioni just-in-time dal CLI", §12) — istruzioni per l'agente su cosa inventariare (TODO, docs/, ADR, git log recente) e come triagiare nel modello ATLAS.
2. L'agente esplora il repo e scrive tramite i normali comandi (`task add`, `card add`, focus) — nessun percorso di scrittura speciale.
3. `atlas doctor` valida il risultato; l'output vive su un branch/worktree dedicato.
4. **Gate umano obbligatorio:** il seed è una proposta; l'umano fa pruning e committa. Mai auto-commit — il seed è l'unico momento in cui garbage-in avvelena tutti i contesti futuri.

**Regola anti-delirio — il seed è *lossy by design*, estrae non migra:**

- **Focus:** 5-10 righe su dove è il progetto *oggi*.
- **Workitem:** solo lavoro aperto e ancora rilevante, con **cap di default (~15)**: se ne servono di più, il triage non è finito. Il materiale storico dei TODO non si importa: resta dov'è.
- **Card:** solo decisioni ancora vincolanti. Gli ADR esistenti **non si copiano mai**: card = hook di 1 riga + `evidence: docs/adr/NNNN-*.md`. ATLAS li indicizza, non li ingerisce.
- **File di history:** esplicitamente esclusi; al più una card "lezioni" (2-3 gotcha ancora attuali) + puntatore.

**Legame con l'MVP:** la Fase 0 (§15) *è* il prototipo di questo brief — il seeding del progetto pilota si fa conversazionalmente con l'agente, e ciò che funziona diventa il testo che `atlas seed` stamperà in Fase 1.

---

## 13. Failure mode dell'architettura

1. **Write-back mancante (il rischio n.1, comportamentale).** L'agente lavora e non aggiorna il ledger → stato stantio → fiducia persa → abbandono (la spirale di morte di ogni sistema di project knowledge, dai TODO agli ADR). Mitigazioni: costo di aggiornamento ~zero (un comando con summary), istruzioni di bootstrap esplicite, e soprattutto **freshness visibile**: `atlas context` dichiara sempre quanto è vecchio lo stato rispetto a git — lo stato stantio è rilevato, mai spacciato per vero. Non-mitigazione deliberata: niente inferenza automatica dello stato dai diff (inaffidabile, e un'inferenza sbagliata presentata come stato è peggio della staleness dichiarata).
2. **Doppia verità.** Se il ledger duplica fatti del codice, divergerà. Mitigazione strutturale: il ledger contiene solo intento/vincoli/stato del lavoro; i fatti restano in codice+git (§7).
3. **Conflitti concorrenti (due agenti/worktree/branch).** Trattazione completa in §12.1. In sintesi: un-file-per-elemento + id hash (collisioni di merge quasi impossibili), binding task↔branch a `task start`, mutazioni di piano confinate al branch di integrazione (policy warn/strict), claims effimeri in `$GIT_COMMON_DIR` per la visibilità tra worktree della stessa macchina, `merge=union` sul log. Caso residuo accettato: agenti su macchine diverse che ignorano il binding advisory → conflitto git normale, visibile e risolvibile.
4. **Rigonfiamento del contesto (deriva di prodotto).** La tentazione di aggiungere sezioni al brief. Mitigazione: budget di token come vincolo testato (test di regressione sulla dimensione dell'output).
5. **Card/decisioni stantie.** Stesso destino degli ADR se non c'è pressione alla revisione. Mitigazione parziale: le card passano per il contesto (vengono lette, quindi possono essere contestate); `atlas doctor` segnala card molto vecchie mai toccate. Rischio accettato: non completamente risolvibile da un tool.
6. **Ledger corrotto/incoerente da edit manuali.** Mitigazione: parsing tollerante + `atlas doctor`; il CLI non presuppone mai di essere l'unico scrittore.
7. **Focus dimenticato.** Il focus dichiarato è l'unico pezzo che può mentire senza che git lo smentisca direttamente. Mitigazione: è minuscolo (si rilegge in 5 secondi) e la freshness lo copre (ultima modifica visibile).

---

## 14. Alternative considerate

1. **Adottare Beads + convenzioni, non costruire nulla.** Beads copre task-DAG, ready-work, compattazione, git-nativo. Si aggiungerebbe: una convenzione per decisioni (MADR) e un focus.md. **Pro:** zero software nuovo, community esistente. **Contro:** manca l'intero livello di compilazione del contesto (il `bd prime` è centrato sui task), manca il modello di card/decisioni, e la dipendenza dal design di un altro progetto (che sta migrando storage, da JSONL a Dolt) limita l'evoluzione; un ATLAS costruito sopra Beads sarebbe di fatto un wrapper attorno a decisioni altrui. **Verdetto: SCARTATA (decisione di prodotto, 2026-08-27).** ATLAS è standalone e possiede il proprio task layer minimale; da Beads si riusano solo le lezioni di design (id hash anti-collisione, ready-detection, summary alla chiusura), non il software. Il costo accettato è reimplementare un task layer semplice — coerente col fatto che i workitem di ATLAS sono deliberatamente più poveri di un issue tracker (§4).
2. **Nessun CLI: solo una convenzione di file + una skill/prompt.** ATLAS come "spec" (layout `.atlas/` + istruzioni): l'agente stesso legge/scrive i file e compila il brief. **Pro:** zero codice, adozione istantanea. **Contro:** la compilazione del contesto diventa essa stessa un consumo di contesto (l'agente legge tutto per riassumere — il problema che volevamo eliminare); nessun output deterministico/testabile; freshness check impraticabile. Bocciata come prodotto finale, ma è un ottimo **esperimento di validazione pre-codice** (provare il formato su un progetto reale prima di scrivere il CLI).
3. **MCP server / daemon con indice.** **Pro:** integrazione ricca, query live. **Contro:** infrastruttura, superficie di tool che consuma contesto, esclude gli agenti senza MCP, contraddice il principio 10. Bocciata per l'MVP; eventuale wrapper MCP *sottile sopra il CLI* in futuro (§16).
4. **Stato derivato al 100% da git (zero ledger).** Sintesi automatica da commit/diff. **Pro:** mai stantio. **Contro:** git registra *cosa è successo*, non *cosa si intende fare* né *perché*; l'inferenza dell'intento è esattamente il tipo di automazione inaffidabile da evitare. Bocciata; sopravvive come componente (GROUND/FRESHNESS).

---

## 15. Confine dell'MVP

**Ipotesi da validare:** "un brief compilato da un ledger minimo riduce materialmente il costo di avvio sessione (token letti e tempo alla prima azione utile) senza degradare la correttezza delle azioni dell'agente."

**Dentro l'MVP (validazione in 2 fasi):**
- **Fase 0 — senza codice (1-2 settimane su un progetto reale):** layout `.atlas/` compilato a mano + istruzioni bootstrap in CLAUDE.md/AGENTS.md. Il seeding iniziale si fa conversazionalmente con l'agente su un repo brownfield reale: è il prototipo del brief di `atlas seed` (§12.2). Misura: l'agente riparte davvero dal ledger? Il formato regge? Che cosa manca nel brief?
- **Fase 1 — CLI minimo:** `init` (layout + installazione del blocco bootstrap in AGENTS.md/CLAUDE.md), `seed` (emissione del brief di curation, §12.2), `context [id] [--json]`, `task add/start/done` (con binding branch e claim, §12.1), `card add/supersede`, `show`, `doctor` (solo freshness). Un binario statico in Go, nessuna dipendenza esterna, nessuna integrazione per-agente oltre al blocco di bootstrap.

**Fuori dall'MVP (esplicitamente):** spec come entità, template per-agente, MCP, hook git, import da TODO/ADR esistenti, multi-repo, web UI, qualsiasi automazione di inferenza dello stato.

**Criteri di successo (protocollo di misura in §17.7, baseline reale acquisito):** tassa di ricostruzione (token dall'avvio alla prima azione produttiva) ridotta di >50% rispetto al baseline TODO.md sullo stesso progetto; **contesto medio per richiesta** (cache read ÷ richieste) e numero di compaction per sessione in calo; il brief tipico sta sotto ~1.500 token; in ≥80% delle sessioni l'agente esegue il write-back senza sollecito umano; zero casi di stato stantio non segnalato dal freshness check. Sul costo totale di sessione l'aspettativa onesta è −10/25% (§17.7).

**Criterio di fallimento (onestà):** se in Fase 0 l'agente ignora sistematicamente il ledger o il write-back non avviene, il problema è comportamentale e nessun CLI lo risolve → fermarsi e ripensare il modello di integrazione prima di scrivere codice.

---

## 16. Evoluzione futura (senza compromettere il minimo)

Ordinate per probabilità di servire davvero; ognuna è additiva, nessuna cambia il modello dati:

1. **Integrazioni per-agente oltre il bootstrap** (`atlas init --integration claude-code|codex|cursor`): slash command/skill wrapper, sul canale già dimostrato da Spec Kit. (L'installazione del blocco bootstrap in AGENTS.md/CLAUDE.md è già nell'MVP; qui si parla solo delle comodità aggiuntive.)
2. ~~**Import leggero**~~ — assorbito da `atlas seed` (§12.2), promosso in Fase 1: il triage assistito del materiale esistente è compito dell'agente guidato dal brief, non di un parser.
3. **Spec di prima classe** — **PROMOSSA E DECISA (2026-08-27):** il flusso di lavoro reale dell'utente è spec-driven e i corpi dei workitem non bastano per intenti grandi. Modello scelto: **spec canoniche viventi** in `.atlas/specs/` — una per capability/area, status draft→active→superseded, aggiornate in place con git come storia dei delta; i workitem si collegano via campo `spec:`; il contesto target include la spec del task. MAI spec-per-feature accumulate (l'anti-pattern verificato di Spec Kit). Dettagli implementativi in PLAN.md §S9.
4. **Hook git opzionali** (post-commit → promemoria di write-back; mai scrittura automatica dello stato).
5. **Wrapper MCP sottile** sopra il CLI, per gli ambienti dove il comando shell non è disponibile.
6. **Query sulla storia** (`atlas log --grep`), sempre on demand.
7. **Multi-progetto / workspace** (aggregazione di più ledger), solo su bisogno reale.

Linea rossa permanente: nessun daemon, nessun DB server, nessun embedding, nessuna scrittura di stato non richiesta esplicitamente da un umano o da un agente.

---

## 17. Questioni aperte (da decidere insieme, non ora)

1. **Nome e posizione della directory:** `.atlas/` (nascosta) vs `atlas/` (visibile)? In git sempre, o supporto a uno strato locale non versionato (stile CLAUDE.local.md)?
2. **Lingua/formato del brief:** testo puro vs markdown leggero? Il budget di 1.500 token è quello giusto? Fisso o adattivo?
3. **Granularità dei workitem:** ATLAS deve scoraggiare task troppo grandi (epic) o è affare dell'utente? Serve `parent` oltre a `blocked_by` e `discovered_from` già nell'MVP?
4. **Summary alla chiusura: obbligatoria o fortemente raccomandata?** Obbligarla garantisce la qualità del RECENT ma aggiunge attrito nel caso "task banale".
5. ~~**Rapporto con Beads**~~ — **RISOLTA (2026-08-27):** ATLAS è standalone, nessun wrapper né dipendenza da Beads; se ne riusano solo le lezioni di design (§14.1).
6. **Stack di implementazione del CLI** — **parzialmente risolta (2026-08-27):** vincolo deciso: binario nativo compilato, niente linguaggi interpretati (il CLI è invocato decine di volte per sessione dagli agenti; l'avvio di un interprete e la gestione delle sue dipendenze sono inaccettabili). Proposta dell'architetto: **Go** (ecosistema nativo della categoria — gh/kubectl/terraform —, cross-compilazione banale per la matrice macOS/Linux/Windows, avvio ~5ms, binario statico singolo, bassa barriera per i contributori; Rust scartato perché il suo vantaggio — performance/memoria — è irrilevante per questo workload). Decisione collegata: interazione con git via **subprocess del `git` di sistema** (come fa `gh`), non libreria embedded — semantica vera di worktree/`--git-common-dir`, e `go-git` ha supporto worktree incompleto. **RISOLTA — confermato Go (2026-08-27).**
7. **Telemetria di validazione — protocollo definito (2026-08-27), da eseguire in Fase 0.** Baseline reale acquisito da 3 sessioni Claude Code (25-29/07) sul progetto di riferimento: costo $180-409/sessione, di cui **>95% è contesto ri-letto/ri-scritto** (cache read 308-678M token ≈ $93-204/sessione; cache write $64-165) contro input fresco <$1 e output 3-4%. Effetto moltiplicativo verificato: ogni token caricato a inizio sessione viene ri-pagato a ogni richiesta successiva (~30k token di ricostruzione × 1.000 richieste ≈ 30M cache read), più le compaction che ri-pagano la ricostruzione (wall-time 1-2 giorni ⇒ cicli ripetuti). **Protocollo (dai transcript JSONL di Claude Code, N sessioni pre vs post-seed sullo stesso repo):** (1) tassa di ricostruzione = token dall'avvio alla prima azione produttiva — qui vale il target −50% del §15; (2) **contesto medio per richiesta** = cache read ÷ n. richieste (la metrica sintetica principale); (3) compaction per sessione; (4) write-back rate; (5) normalizzazione per ora API o righe cambiate. **Aspettativa onesta sul costo totale: −10/25%** (le letture di codice sono lavoro necessario e restano), ovvero $30-100 su sessioni come quelle del baseline; il target −50% riguarda la sola tassa di avvio. Il beneficio qualitativo (meno context rot, meno compaction) è atteso superiore a quello economico.
   **Baseline misurato dai transcript (2026-08-27, repo migration-toolkit-v2, sessione principale 03/08: 124 richieste, 22h wall):** contesto medio per richiesta **157.6k token**; prima azione produttiva alla **9ª richiesta** con **~49.5k token** di contesto accumulato (~392k cache read spesi in avvio); **TODO.md ≈ 12.4k token a lettura, riletto 3 volte nella stessa sessione** (~37k totali — la firma del problema: refresh dello stato via ri-lettura della storia, poi trascinata in finestra per ~115 richieste ≈ 1.4M token di cache read); AGENTS.md ~0.6k (già nella taglia giusta). Obiettivo concreto: `atlas context` ≤1.5k sostituisce la componente-stato dei ~50k di avvio e le refresh da 12.4k (−88% su quella componente). **Scan completo di ~/.claude/projects (29 progetti, 8 sessioni con dati, 5 con ≥30 richieste):** mediane — contesto medio per richiesta **127.6k token** (range 64.5k-286.4k), prima azione produttiva alla **13ª richiesta** (range 1-27), contesto a quel punto **66.6k token** (range 49.5k-93.6k). Il pattern è trasversale ai progetti, non specifico di migration-toolkit-v2; caso peggiore: sessione ai-harness a 286k/richiesta, prima azione alla 27ª richiesta. Caveat: le sessioni di luglio del baseline economico non sono più su disco (retention dei transcript o altra macchina) — il baseline economico resta quello degli screenshot, quello strutturale è misurato su 5 sessioni.
8. **Il nome `card`:** regge per l'unione decisione+conoscenza, o confonde? Alternative: `note`, `fact`, `rule`.
9. **Concorrenza (§12.1):** la policy sulle mutazioni di piano fuori dal branch di integrazione deve essere `warn` o `strict` di default? Il layer di claims in `$GIT_COMMON_DIR` entra nell'MVP (Fase 1) o slitta, lasciando all'MVP il solo binding task↔branch? Quali branch contano come "di integrazione" (develop? main? configurabile)?

---

*Fonti: ricerca condotta il 2026-08-27 su fonti primarie (repo GitHub, documentazione ufficiale Anthropic/OpenAI/MADR/agents.md, recensioni indipendenti). Citazioni puntuali disponibili nei rapporti di ricerca della sessione.*
