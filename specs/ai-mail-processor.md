---
status: implemented
priority: medium
tags: [go, ai, email, imap, microsoft365]
created: 2026-01-19
completed: 2026-01-19
---

# AI Mail Processor

## Bakgrund

Behovet av att automatiskt hantera inkommande mail baserat på AI-regler definierade i en markdown-fil. Istället för att manuellt sortera och svara på mail kan en AI-modell analysera varje mail och utföra fördefinierade åtgärder baserat på regler.

**Status: IMPLEMENTERAD** ✅

## Beskrivning

En Go-applikation som:
1. Ansluter till en IMAP-server och hämtar mail från en specificerad mapp
2. Parsar en markdown-fil med regler och instruktioner för AI-modellen
3. Skickar varje mail till en AI-modell tillsammans med reglerinstruktionerna
4. Exekverar AI:ns föreslagna åtgärder (svara, flytta, arkivera, etc.)
5. Loggar alla handlingar

### Målstruktur

```
src/go/
├── main.go
├── go.mod
├── go.sum
├── config/
│   └── config.go          # Konfigurationshantering
├── imap/
│   └── client.go          # IMAP-klient
├── rules/
│   ├── parser.go          # Markdown-regelparser
│   └── rules.go           # Regeldatastrukturer
├── ai/
│   └── provider.go        # AI-provider (OpenAI/Anthropic/Ollama)
├── actions/
│   ├── executor.go        # Åtgärdexekverare
│   ├── reply.go           # Svara på mail
│   ├── move.go            # Flytta mail
│   └── archive.go         # Arkivera mail
├── logging/
│   └── logger.go          # Loggning
└── examples/
    └── rules.md           # Exempel på regelfil
```

### Markdown-regelformat

```markdown
# AI Mail Agent - Regler

## Systeminstruktion
Du är en intelligent mail-assistent. Analysera inkommande mail och utför lämpliga åtgärder baserat på nedanstående regler.

## Regler

### Regel: Nyhetsbrev
Villkor: `subject contains "nyhetsbrev" OR from contains "newsletter"`
Åtgärd: 
  - move: "Newsletter"
  - mark_read: true

### Regel: Supportförfrågan  
Villkor: `subject contains "support" OR subject contains "hjälp"`
Åtgärd:
  - reply: "Tack för ditt meddelande. Vi återkommer inom 24 timmar."
  - add_label: "Support"
  - mark_read: false

### Regel: Faktura
Villkor: `subject contains "faktura" OR attachment exists`
Åtgärd:
  - move: "Invoices"
  - add_label: "Billing"
  - forward: "ekonomi@example.com"

### Regel: VIP-kund
Villkor: `from in vip_contacts`
Åtgärd:
  - priority: high
  - notify: true
  - move: "VIP"
```

### Åtgärder som stöds

| Åtgärd | Beskrivning |
|--------|-------------|
| `reply` | Skicka ett svar baserat på mall eller AI-genererat |
| `move` | Flytta mailet till annan mapp |
| `copy` | Kopiera mailet till annan mapp |
| `archive` | Arkivera mailet |
| `delete` | Ta bort mailet |
| `mark_read` | Markera som läst/oläst |
| `mark_important` | Markera som viktigt |
| `forward` | Vidarebefordra till annan adress |
| `add_label` | Lägg till etikett/tagg |
| `priority` | Sätt prioritet |
| `notify` | Skicka notifikation om åtgärd |

### Konfigurationsfil (config.yaml)

```yaml
mail:
  server: "outlook.office365.com"
  port: 993
  username: "user@example.com"
  password_env: "MAIL_PASSWORD"
  folder: "INBOX"
  check_interval: "5m"
  use_tls: true

ai:
  provider: "openai-compatible"
  endpoint: "http://localhost:8080/v1"  # GTP-5.6 Luna API endpoint
  model: "gpt-5.6-luna"
  api_key_env: "LUNA_API_KEY"

rules:
  file: "rules.md"

logging:
  level: "info"
  file: "mail-agent.log"

fallback:
  enabled: true
  notify_email: "personal.email@gmail.com"
  smtp_server: "smtp.office365.com"
  smtp_port: 587
  smtp_username: "user@example.com"
  smtp_password_env: "SMTP_PASSWORD"
  from_email: "user@example.com"
```

## Acceptanskriterier

- [ ] Ansluter till IMAP-server med TLS
- [ ] Parsar markdown-regelfil korrekt
- [ ] Skickar mail till AI-modell med rätt kontext
- [ ] Exekverar minst 3 olika åtgärdstyper
- [ ] Loggar alla åtgärder
- [ ] Hanterar fel gracefully
- [ ] Stödjer både OpenAI och Ollama
- [ ] Konfiguration via YAML-fil
- [ ] CLI-flaggor för att styra beteende

## Tekniska beslut

- **IMAP**: github.com/emersion/go-imap med imapclient sub-package
- **Markdown-parsing**: Egen enkel parser
- **AI**: OpenAI-kompatibelt API för GTP-5.6 Luna
- **Logging**: Standard Go slog med strukturerad JSON-output

## Kända begränsningar

- Body-parsing är förenklad (ingen HTML-stripping för alla fall)
- SMTP fallback använder basic auth
- Ingen rate limiting för AI-anrop

## Framtida utökningar

- Schemaläggning (cron-liknande)
- Feedback-loop (AI lär sig från manuella korrigeringar)
- WebUI för att övervaka och godkänna åtgärder
- Rate limiting för att undvika att överskrida API-kvoter
