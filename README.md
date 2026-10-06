# MailMaster

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

## Konfiguration

Exempelkonfiguration finns i `src/go/config/config.yaml`.

## API Endpoints

| Metod | Path                                              | Beskrivning          |
| ----- | ------------------------------------------------- | -------------------- |
| GET   | `/api/v1/accounts`                                | Lista konton         |
| GET   | `/api/v1/accounts/{name}/folders`                 | Lista mappar         |
| GET   | `/api/v1/accounts/{name}/folders/{folder}/emails` | Lista mail           |
| POST  | `/api/v1/accounts/{name}/process`                 | Processa mail med AI |
| GET   | `/api/v1/rules`                                   | Hämta regler         |
| PUT   | `/api/v1/rules`                                   | Spara regler         |

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
└── app/
    ├── pages/        # Sidor
    ├── components/   # Komponenter
    └── composables/  # Vue composables
```
