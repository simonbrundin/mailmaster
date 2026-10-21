# <!-- wiki:start -->
## Kunskaps-wiki

Det här repot delar en gemensam kunskaps-wiki med alla andra repor.
Wikin ligger i `~/wiki`.

**Två regler:**

1. **Läs wikin innan du frågar användaren.** Svaret finns ofta redan
   dokumenterat - leta först.
2. **Skriv en not i wikin inbox när något stort händer.** Incident, beslut,
   ny feature eller en icke-obvious bugg.

```bash
wikictl search "bigram"                  # sök i hela wikin
wikictl search "fel" --project <repo>    # sök inom ett projekt
wikictl note --title "Rubrik" --tags "bug" --body "Vad som hände och varför."
wikictl ingest                          # flytta inboxen in i grafen
```

Anteckningar taggas automatiskt med `projekt:<repo>`.
<!-- wiki:end -->
