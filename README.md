# MailMaster

> **Flyttad till [pappa](../pappa) (2026-11-16).** Go-klienten ligger i
> `pappa/src/go`, sidorna i `pappa/src/nuxt/app/pages/mail` och klustret
> deployas som `mailagent`. Det här repot är arkiv - ändra i pappa.

AI-driven e-posthantering. Ansluter till IMAP, skickar mail till en AI-modell tillsammans med regler definierade i markdown, och exekverar AI:ns föreslagna åtgärder.

## Arkitektur

MailMaster har två huvudkomponenter:

- `src/go/` (Go backend) - IMAP, AI, SMTP, MCP-server, REST API
- `src/nuxt/` (Nuxt frontend) - SPA för email-hantering

## Körlägen

Go-backend har tre körlägen:

| Läge | Kommando                        | Beskrivning                 |
| ---- | ------------------------------- | --------------------------- |
| CLI  | `./mailagent --once --limit 10` | Processera mail och avsluta |
| API  | `./mailagent --api :8401`       | REST API för frontend       |
| MCP  | `./mailagent --mcp`             | MCP-server för AI-agenter   |

## Regler

Regler definieras i markdown och tolkas av AI:n:

```markdown
# AI Mail Agent - Regler

## Regel: Nyhetsbrev

Villkor: subject contains "nyhetsbrev"
Åtgärd: read

## Regel: Svara automatiskt

Villkor: subject contains "order"
Åtgärd: reply
Svara: Tack för ditt meddelande!
```

AI:t svarar med vilken regel som matchade (`REGEL:`), vilka åtgärder som
rekommenderas (`ÅTGÄRD:`) och varför (`MOTIVERING:`).

## Konfiguration

Exempelkonfiguration finns i `src/go/config/config.yaml`.

## API Endpoints

| Metod | Path                                              | Beskrivning          |
| ----- | ------------------------------------------------- | -------------------- |
| GET   | `/api/v1/accounts`                                | Lista konton         |
| GET   | `/api/v1/accounts/{name}/folders`                 | Lista mappar         |
| GET   | `/api/v1/accounts/{name}/folders/{folder}/emails` | Lista mail           |
| POST  | `/api/v1/accounts/{name}/process`                 | Processa mail med AI (utför åtgärder direkt) |
| POST  | `/api/v1/accounts/{name}/analyze`                 | Analysera mail med AI utan att utföra något |
| POST  | `/api/v1/accounts/{name}/decision`                | Godkänn (`approve`) eller avböj (`deny`) en föreslagen åtgärd |
| GET   | `/api/v1/rules`                                   | Hämta regler         |
| PUT   | `/api/v1/rules`                                   | Spara regler         |

### Granskning innan åtgärd

UI-fönstret använder ett tvåstegsflöde så att inget görs med mailet utan att
du godkänner det:

1. **Analysera** (`/analyze`) - AI:t läser varje mejl, avgör vilken regel som
   matchar och vilka åtgärder som rekommenderas. Inget utförs, resultatet
   sparas på servern.
2. **Beslut** (`/decision`) - du trycker *OK* eller *Neka* bredvid varje mejl.
   *OK* utför de föreslagna åtgärderna (med de argument AI:t föreslog),
   *Neka* lämnar mejlet orört.

## Kameo utbetalningar

Nuxt-sidan `/kameo-utbetalningar` loggar in på Kameo och hämtar bolagets lån.
Man skriver ett lånnummer och får tillbaka om tillgången är **kortsiktig**
(mindre än 12 månader kvar att löpa) eller **långsiktig**.

Inloggningen sker server-side i Nitro mot Kameos JSON-API
(`api.kameo.se/v1`) och kräver e-post, lösenord och engångskod (TOTP).

### Inloggningsuppgifter

Uppgifterna (e-post, lösenord och TOTP-hemlighet) finns i 1Password-objektet
`Kameo - Michael Brundin AB` i vaulten `Private`, och synkas till en
**SOPS-krypterad** fil med age.

**Så här får du in logga in:**

1. Synka och kryptera uppgifterna från 1Password (körs på värden, där
   1Password-appen finns):

   ```bash
   cd src/nuxt && bun run kameo:sync
   ```

   Skriptet skriver `environments/dev/kameo.env.enc` (krypterat med SOPS +
   age, läges 600) med `KAMEO_EMAIL`, `KAMEO_PASSWORD` och
   `KAMEO_TOTP_SECRET`. Filen är krypterad och kan checkas in i git.

2. Starta om Nuxt:

   ```bash
   # Docker - dekryptera till .env-filen och skapa om containern
   cd src/nuxt && bun run kameo:decrypt
   docker compose up -d --force-recreate nuxt

   # eller kör direkt på värden (läser .enc-filen och dekrypterar i minnet)
   cd src/nuxt && bun run dev
   ```

Serverprocessen letar efter uppgifterna i den ordning: miljövariabler ->
`kameo.env` (klartext) -> `kameo.env.enc` (dekrypteras i minnet) -> 1Password
via `op`-CLI:t. Därför fungerar Docker-läget utan `op` i containern, och en
värdkörning kan läsa direkt från 1Password.

| Miljövariabel          | Standardvärde                        | Beskrivning                        |
| ---------------------- | ------------------------------------ | ---------------------------------- |
| `KAMEO_CREDENTIALS_FILE` | `environments/dev/kameo.env`        | Klartextfil (för docker env_file)  |
| `KAMEO_ENCRYPTED_FILE` | `environments/dev/kameo.env.enc`      | SOPS-krypterad fil                 |
| `SOPS_AGE_RECIPIENT`   | infra-repoets `age1mv3yrg…`          | Mottagarnyckel för kryptering      |
| `SOPS_AGE_KEY_FILE`    | `~/.config/sops-age/keys.txt`        | Privat age-nyckel                  |
| `KAMEO_OP_ITEM`        | `Kameo - Michael Brundin AB`          | 1Password-objektets titel eller id  |
| `KAMEO_OP_VAULT`       | `Private`                             | Vaulten som objektet ligger i      |
| `KAMEO_EMAIL`          | –                                     | E-postadress                       |
| `KAMEO_PASSWORD`       | –                                     | Lösenord                           |
| `KAMEO_TOTP_SECRET`    | –                                     | base32-hemlighet för TOTP          |

TOTP-koden genereras lokalt från hemligheten, så `op` behöver inte anropas
för varje inloggning.

### Endpoint

| Metod | Path                    | Beskrivning                                             |
| ----- | ----------------------- | ------------------------------------------------------- |
| POST  | `/api/kameo/loans`      | Sök lånnummer, svarar med kortsiktig/långsiktig-klassning |

## Utveckling

### Projektstruktur

```
src/go/
├── main.go           # Entry point
├── config/           # Konfiguration
├── imap/             # IMAP-klient
├── ai/               # AI-provider (OpenAI-kompatibelt)
│   ├── provider.go   # API-klient
│   ├── response.go   # Response-parsing
│   └── auth.go       # OAuth2-autentisering
├── rules/            # Reglerparser
│   ├── rules.go      # Markdown-parsing
│   └── email.go      # Epost-hjälpfunktioner
├── actions/          # Åtgärdsexekvering
│   ├── executor.go    # Action executor
│   └── smtp.go       # SMTP-klient
├── server/           # REST API
├── mcp/              # MCP-server
│   ├── server.go     # Protocol handler
│   ├── types.go      # MCP types
│   └── tools.go      # Tool schemas
└── sasl/             # SASL-mekanismer

src/nuxt/
├── nuxt.config.ts    # Nuxt-konfiguration
├── server/           # Nitro (server-side API)
│   ├── api/          # API-routes
│   └── utils/        # Server-hjälpare (Kameo-klient, TOTP)
└── app/
    ├── pages/        # Sidor
    ├── components/   # Komponenter
    └── composables/  # Vue composables
```
