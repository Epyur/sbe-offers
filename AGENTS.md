# AGENTS.md тАФ sbe-offers (LogicTEAM.╨Ъ╨Я)

╨Ъ╨╛╨╝╨╝╨╡╤А╤З╨╡╤Б╨║╨╕╨╡ ╨┐╤А╨╡╨┤╨╗╨╛╨╢╨╡╨╜╨╕╤П: ╨║╨░╤В╨░╨╗╨╛╨│ ╤Б╨╕╤Б╤В╨╡╨╝ ╨╕ ╨╝╨░╤В╨╡╤А╨╕╨░╨╗╨╛╨▓, ╨║╨╛╨╜╤Д╨╕╨│╤Г╤А╨░╤В╨╛╤А ╤Д╨╛╤А╨╝╤Л ╨Ъ╨Я,
╤Б╨▒╨╛╤А╨║╨░ ╨┐╤А╨╡╨┤╨╗╨╛╨╢╨╡╨╜╨╕╨╣ ╨▓ ╨┐╨╗╨░╨│╨╕╨╜╨╡ ╨╕ ╤З╨╡╤А╨╡╨╖ **╤Б╨╛╨▒╤Б╤В╨▓╨╡╨╜╨╜╨╛╨│╨╛ Telegram-╨▒╨╛╤В╨░**, ╤Н╨║╤Б╨┐╨╛╤А╤В ╨▓
DOCX. ╨Ы╨╕╨╜╨╡╨╣╨╜╤Л╨╡ ╨╝╨╡╨╜╨╡╨┤╨╢╨╡╤А╤Л ╤А╨░╨▒╨╛╤В╨░╤О╤В ╨▓ ╨▒╨╛╤В╨╡, ╨░╨┤╨╝╨╕╨╜╨╕╤Б╤В╤А╨░╤В╨╛╤А ╨╜╨░╤Б╤В╤А╨░╨╕╨▓╨░╨╡╤В ╨║╨░╤В╨░╨╗╨╛╨│ ╨╕
╤Д╨╛╤А╨╝╤Г ╨▓ ╨┐╨╗╨░╨│╨╕╨╜╨╡.

**╨С╤Н╨║ тАФ ╨▓ ╤Н╤В╨╛╨╣ ╨╢╨╡ ╨┐╨░╨┐╨║╨╡**: `offers-service/` (Go, API) ╨╕ `offers-bot/` (Node,
Telegram long polling) тАФ ╨╜╨░ ╨▓╨╡╤В╨║╨╡ `backend` (main тАФ ╤З╨╕╤Б╤В╤Л╨╣ ╤А╨╡╨╗╨╕╨╖╨╜╤Л╨╣ ╤Б╤А╨╡╨╖ ╨║╨╛╨┤╨░
╨┐╨╗╨░╨│╨╕╨╜╨░, ╨▒╨╡╨╖ ╨▒╤Н╨║╨░; ╤Б╨╝. ╨┐╤А╨░╨▓╨╕╨╗╨╛ ┬л╨С╤Н╨║╨╕ ╨▓ ╨┐╨░╨┐╨║╨░╤Е ╨┐╨╗╨░╨│╨╕╨╜╨╛╨▓┬╗ ╨▓ ╨║╨╛╤А╨╜╨╡╨▓╨╛╨╝
`plugins/AGENTS.md`).

╨Ф╨╕╨╖╨░╨╣╨╜ тАФ `docs/superpowers/specs/2026-10-26-sbe-offers-design.md`,
╨┐╨╗╨░╨╜ тАФ `docs/superpowers/plans/2026-10-26-sbe-offers.md`.

## ╨Э╨░╨╖╨╜╨░╤З╨╡╨╜╨╕╨╡ (╤В╨╡╨║╤Г╤Й╨╡╨╡)

- **╨Ъ╨░╤В╨░╨╗╨╛╨│** (`offers_catalog`) тАФ ╨║╨░╤А╤В╨╛╤З╨║╨╕ ╤Б╨╕╤Б╤В╨╡╨╝/╨╝╨░╤В╨╡╤А╨╕╨░╨╗╨╛╨▓/╤А╨░╨▒╨╛╤В: ╨╜╨░╨╕╨╝╨╡╨╜╨╛╨▓╨░╨╜╨╕╨╡,
  ╤В╨╕╨┐, ╨╡╨┤╨╕╨╜╨╕╤Ж╨░, ╨▒╨░╨╖╨╛╨▓╨░╤П ╤Ж╨╡╨╜╨░, ╨╛╨┐╨╕╤Б╨░╨╜╨╕╨╡. ╨Т╨╡╨┤╤С╤В ╤В╨╛╨╗╤М╨║╨╛ admin; ╨░╤А╤Е╨╕╨▓╨╕╤А╤Г╤О╤В╤Б╤П, ╨░ ╨╜╨╡
  ╤Г╨┤╨░╨╗╤П╤О╤В╤Б╤П (╨╜╨░ ╨╜╨╕╤Е ╤Б╤Б╤Л╨╗╨░╤О╤В╤Б╤П ╤Б╤В╨░╤А╤Л╨╡ ╨┐╤А╨╡╨┤╨╗╨╛╨╢╨╡╨╜╨╕╤П).
- **╨д╨╛╤А╨╝╨░ ╨Ъ╨Я** (`offers_form`) тАФ ╨┐╨╗╨╡╨╣╤Б╤Е╨╛╨╗╨┤╨╡╤А╤Л + ╨▒╨╗╨╛╨║╨╕ ╨┤╨╛╨║╤Г╨╝╨╡╨╜╤В╨░ (╨░╨▒╨╖╨░╤Ж,
  ╨╖╨░╨│╨╛╨╗╨╛╨▓╨╛╨║, ╤Б╨┐╨╕╤Б╨╛╨║, ╤В╨░╨▒╨╗╨╕╤Ж╨░) + ╨╛╨▒╤П╨╖╨░╤В╨╡╨╗╤М╨╜╤Л╨╡ ╨┐╨╛╨╗╤П. ╨а╨╡╨┤╨░╨║╤В╨╛╤А тАФ ╤Б╨▓╨╛╨╣ ╨▒╨╗╨╛╨║-╤А╨╡╨┤╨░╨║╤В╨╛╤А
  ╨┐╨╛ ╨╛╨▒╤А╨░╨╖╤Ж╤Г ╨║╨╛╨╜╤Д╨╕╨│╤Г╤А╨░╤В╨╛╤А╨░ ╨┐╤А╨╛╤В╨╛╨║╨╛╨╗╨╛╨▓ ╨Ы╨Ш╨Ь╨б.
- **╨Ъ╨Я** (`offers_offer`) тАФ ╤Б╨╜╨╕╨╝╨╛╨║: ╨┐╨╛╨╖╨╕╤Ж╨╕╨╕ ╤Б ╤Ж╨╡╨╜╨╛╨╣ ╨╜╨░ ╨╝╨╛╨╝╨╡╨╜╤В ╨┤╨╛╨▒╨░╨▓╨╗╨╡╨╜╨╕╤П,
  ╨╖╨╜╨░╤З╨╡╨╜╨╕╤П ╤Д╨╛╤А╨╝╤Л, ╨╕╤В╨╛╨│, ╤Б╤В╨░╤В╤Г╤Б `draft`/`final`, ╨╜╨╛╨╝╨╡╤А `{NNN}/{yyyy}` ╤Б╨║╨▓╨╛╨╖╨╜╨╛╨╣ ╨┐╨╛
  ╨│╨╛╨┤╤Г. ╨Т╨╕╨┤╨╕╨╝╨╛╤Б╤В╤М: admin ╨▓╨╕╨┤╨╕╤В ╨▓╤Б╨╡, ╨╛╤Б╤В╨░╨╗╤М╨╜╤Л╨╡ тАФ ╤Б╨▓╨╛╨╕.
- **Telegram-╨▒╨╛╤В** тАФ ╨▓╤Е╨╛╨┤ ╨┐╨╛ e-mail ╨╕ ╨║╨╛╨┤╤Г (╤Б╨░╨╝╨╛╨╖╨░╨┐╤А╨╛╤Б ╨╜╨░ ╨┐╨╛╤З╤В╤Г ╨╗╨╕╨▒╨╛ ╨║╨╛╨┤ ╨╛╤В
  ╨░╨┤╨╝╨╕╨╜╨╕╤Б╤В╤А╨░╤В╨╛╤А╨░), ╤Б╨▒╨╛╤А╨║╨░ ╨Ъ╨Я ╨╕╨╖ ╨║╨░╤В╨░╨╗╨╛╨│╨░ ╨╕ ╤Б╨▓╨╛╨▒╨╛╨┤╨╜╤Л╨╝╨╕ ╤Б╤В╤А╨╛╨║╨░╨╝╨╕, DOCX ╤Д╨░╨╣╨╗╨╛╨╝ ╨▓
  ╤З╨░╤В ╨╕╨╗╨╕ ╤Б╤Б╤Л╨╗╨║╨╛╨╣ ╨┐╨╛ ╨║╨╜╨╛╨┐╨║╨╡.
- **╨в╨╛╤З╨║╨░ ╨▓╤Е╨╛╨┤╨░** тАФ ╨╝╨░╨│╨░╨╖╨╕╨╜: ┬л╨г╤Б╤В╨░╨╜╨╛╨▓╨╗╨╡╨╜╨╜╤Л╨╡ тЖТ ╨Ю╤В╨║╤А╤Л╤В╤М┬╗ (`publishService('sbe-offers', {open})`).

## ╨Р╤А╤Е╨╕╤В╨╡╨║╤В╤Г╤А╨░

```
sbe-offers (╨┐╨╗╨░╨│╨╕╨╜)  тФАтФАJWT app_id=offersтФАтФАтЦ║  offers-service (Go)  тФАтФАтЦ║  Postgres offers
                                                    тФВ                    тФФтФАтЦ║ S3 sbe-offers (DOCX)
offers-bot (Node)  тФАтФАX-Service-SecretтФАтФАтЦ║  offers-service
        тФФтФАтФА long polling ╤З╨╡╤А╨╡╨╖ SOCKS5-╤Б╨░╨╣╨┤╨║╨░╤А offers-nl-proxy (nl_egress_proxy)
```

- ╨Ы╨╕╤З╨╜╨╛╤Б╤В╤М тАФ ╨╛╨▒╤Й╨╕╨╣ `auth-service`: email; ╤А╨╛╨╗╨╕ `viewer` < `editor` < `admin` тАФ
  ╨╕╨╖ ╤Ж╨╡╨╜╤В╤А╨░╨╗╤М╨╜╨╛╨╣ ╤В╨░╨▒╨╗╨╕╤Ж╤Л ╨┐╤А╨░╨▓ (╤Б╨╝. `authperms.go`).
- **╨Ъ╨╛╨┤╤Л ╨▓╤Е╨╛╨┤╨░, ╨┐╤А╨╕╨▓╤П╨╖╨║╨╕ chat_id ╨╕ ╤А╨╛╨╗╨╕ ╨▒╨╛╤В╨░ ╨╢╨╕╨▓╤Г╤В ╨▓ `offers-service`**
  (`bot_codes`, `offers_bot_link`, `offers_access`) тАФ auth-service ╨╜╨╡ ╨┐╤А╨░╨▓╨╕╤В╤Б╤П.
- Telegram ╤Б VDS ╨▓ ╨а╨д ╨╜╨╡╨┤╨╛╤Б╤В╤Г╨┐╨╡╨╜ ╨╜╨░╨┐╤А╤П╨╝╤Г╤О (╨┤╨╕╨░╨┐╨░╨╖╨╛╨╜╤Л AS62041 ╨╖╨░╨▒╨╗╨╛╨║╨╕╤А╨╛╨▓╨░╨╜╤Л
  ╤Е╨╛╤Б╤В╨╡╤А╨╛╨╝) тАФ ╨┐╨╛╨╗╨╗╨╡╤А ╤Е╨╛╨┤╨╕╤В ╤З╨╡╤А╨╡╨╖ SOCKS5, ╨║╨░╨║ `portal-embed`.

## ╨б╤В╤А╤Г╨║╤В╤Г╤А╨░ ╨┐╨╗╨░╨│╨╕╨╜╨░

| ╨д╨░╨╣╨╗ | ╨з╤В╨╛ ╤Н╤В╨╛ |
|---|---|
| `src/main.ts` | `SbeOffersPlugin`: ╨╜╨░╤Б╤В╤А╨╛╨╣╨║╨╕, view, publishService, ╨╜╨╛╨▓╨╛╤Б╤В╤М ╨▓ ╨ж╨г╨Я |
| `src/constants.ts` | `OFFERS_VIEW_TYPE`, `OFFERS_APP_ID`, `OFFERS_PLUGIN_ID` |
| `src/types/offers.ts` | `CatalogItem`, `OfferForm`, `Offer`, `OfferItem`, `AccessEntry`, `Role` |
| `src/types/settings.ts` | `OffersSettings { apiUrl, lastAnnouncedVersion }` |
| `src/services/offers.service.ts` | HTTP ╨║ `/api/offers/*`, `OffersError`, `exportDocx` |
| `src/ui/offers-view.ts` | ╤Д╨░╤Б╨░╨┤: ╤В╨╛╨┐╨▒╨░╤А + ╤Б╨░╨╣╨┤╨▒╨░╤А + ╨║╨╛╨╜╤В╨╡╨╜╤В |
| `src/ui/offers-list.ts` | ┬лЁЯз╛ ╨Ъ╨Я┬╗: ╤Б╨┐╨╕╤Б╨╛╨║, ╨║╨░╤А╤В╨╛╤З╨║╨░, ╤А╨╡╨┤╨░╨║╤В╨╛╤А ╤Б╨╛╤Б╤В╨░╨▓╨░, DOCX, ╤Б╤Б╤Л╨╗╨║╨░ |
| `src/ui/catalog-editor.ts` | ┬лЁЯЧВ ╨Ъ╨░╤В╨░╨╗╨╛╨│┬╗: ╤В╨░╨▒╨╗╨╕╤Ж╨░, ╨╝╨╛╨┤╨░╨╗╨║╨░ ╨║╨░╤А╤В╨╛╤З╨║╨╕ |
| `src/ui/form-editor.ts` | ┬лтЪЩя╕П ╨д╨╛╤А╨╝╨░ ╨Ъ╨Я┬╗: ╨┐╨╛╨╗╤П, ╨▒╨╗╨╛╨║╨╕, ╨╛╨▒╤П╨╖╨░╤В╨╡╨╗╤М╨╜╤Л╨╡ |
| `src/ui/block-editor.ts` | ╨▒╨╗╨╛╨║-╤А╨╡╨┤╨░╨║╤В╨╛╤А ╤Б ╤З╨╕╨┐╨░╨╝╨╕ `{{╨┐╨╛╨╗╨╡}}` |
| `src/ui/access-editor.ts` | ┬лЁЯСд ╨С╨╛╤В ╨╕ ╨┤╨╛╤Б╤В╤Г╨┐┬╗: ╨║╨╛╨┤╤Л, ╤А╨╛╨╗╨╕ ╨▓╨╛╤И╨╡╨┤╤И╨╕╤Е |
| `src/ui/help.ts` | ╤В╨╡╨║╤Б╤В ╤Б╨┐╤А╨░╨▓╨║╨╕ (╨▓╤И╨╕╤В ╨▓ ╨┐╨╗╨░╨│╨╕╨╜) |
| `src/styles.css` | ╨║╨╗╨░╤Б╤Б╤Л `tn-offers-*` |

## ╨б╤В╤А╤Г╨║╤В╤Г╤А╨░ ╤Б╨╡╤А╨▓╨╡╤А╨░

`offers-service/`: `main.go` (╤А╨╛╤Г╤В╤Л, ╨╝╨╕╨│╤А╨░╤Ж╨╕╨╕), `jwt.go`, `register.go`,
`authperms.go` (╨║╨╛╨┐╨╕╤П ╨╕╨╖ ╨┤╤А╤Г╨│╨╕╤Е ╤Б╨╡╤А╨▓╨╕╤Б╨╛╨▓ тАФ ╨┐╤А╨░╨▓╨╕╤В╤М ╤Б╤А╨░╨╖╤Г ╨▓╨╛ ╨▓╤Б╨╡╤Е),
`permissions.go`, `catalog.go`, `form.go`, `offers.go`, `docx.go`,
`access.go`, `botapi.go`, `s3.go`, `email.go`, `static.go`, `template.go`,
`static/openapi.yaml`.

`offers-bot/`: `src/main.ts`, `src/poller.ts` (╤В╤А╨░╨╜╤Б╨┐╨╛╤А╤В ╨╕ ╨╝╨░╤А╤И╤А╤Г╤В╨╕╨╖╨░╤Ж╨╕╤П),
`src/telegram.ts` (Bot API ╤З╨╡╤А╨╡╨╖ SOCKS), `src/session.ts` (╨╝╨░╤И╨╕╨╜╨░ ╤Б╨╛╤Б╤В╨╛╤П╨╜╨╕╨╣
╨▓╤Е╨╛╨┤╨░), `src/flow.ts` (╤Б╨▒╨╛╤А╨║╨░ ╨Ъ╨Я), `src/api.ts`, `Dockerfile`.

## ╨Э╨░╤Б╤В╤А╨╛╨╣╨║╨╕ (data.json)

`apiUrl` (default `https://epyur.fvds.ru`), `lastAnnouncedVersion`.

## ╨Я╨╡╤А╨╡╨╝╨╡╨╜╨╜╤Л╨╡ ╨╛╨║╤А╤Г╨╢╨╡╨╜╨╕╤П (╤Б╨╡╤А╨▓╨╡╤А)

`DATABASE_URL`, `PORT`, `JWT_SECRET`, `OFFERS_APP_ID` (`offers`),
`OFFERS_APP_NAME`, `OFFERS_OWNER_EMAIL`, `OFFERS_SERVICE_SECRET`,
`OFFERS_BOT_SECRET`, `AUTH_SERVICE_URL`, `S3_ENDPOINT`/`S3_ACCESS_KEY`/
`S3_SECRET_KEY`/`S3_BUCKET` (`sbe-offers`), `SMTP_*`.
╨С╨╛╤В: `TELEGRAM_BOT_TOKEN` (╤Б╨▓╨╛╨╣, ╨╕╨╖ @BotFather), `TELEGRAM_SOCKS_PROXY`,
`OFFERS_API_URL`, `OFFERS_BOT_SECRET`, `OFFERS_BOT_RATE_LIMIT`.

## ╨Я╤А╨╛╨▓╨╡╤А╨║╨╕

```
npx tsc --noEmit && npm run build          # ╨┐╨╗╨░╨│╨╕╨╜
cd offers-service && gofmt -l . && go vet ./... && go test ./...
cd offers-bot && npm run typecheck && npm run build && node --test "src/flow.test.ts"
node plugins/scripts/check_offers_openapi.mjs
```

## ╨Я╤А╨░╨▓╨╕╨╗╨░

- `catch(e: unknown)` + `errorMessage()`; `requestUrl()`; `window.setTimeout()`;
  ╨▒╨╡╨╖ `any`; UI ╨╜╨░ ╤А╤Г╤Б╤Б╨║╨╛╨╝; ╨║╨╗╨░╤Б╤Б╤Л `tn-offers-*` ╨╜╨░ ╤Б╨╡╨╝╨░╨╜╤В╨╕╤З╨╡╤Б╨║╨╕╤Е ╤В╨╛╨║╨╡╨╜╨░╤Е.
- ╨Ъ╨╛╨╝╨╝╨╕╤В╤Л/╨┐╤Г╤И╨╕ тАФ ╤В╨╛╨╗╤М╨║╨╛ ╨┐╨╛ ╤П╨▓╨╜╨╛╨╣ ╨║╨╛╨╝╨░╨╜╨┤╨╡ ╨┐╨╛╨╗╤М╨╖╨╛╨▓╨░╤В╨╡╨╗╤П.
- **┬л╨д╨╕╨║╤Б╨╕╤А╤Г╨╣┬╗ = ╨┐╨╛╨┤╨╜╤П╤В╤М ╨▓╨╡╤А╤Б╨╕╤О (+0.0.1 ╨▓ `manifest.json` ╨╕ `package.json`),
  ╨╛╨▒╨╜╨╛╨▓╨╕╤В╤М ╨┤╨╛╨║╤Г╨╝╨╡╨╜╤В╨░╤Ж╨╕╤О, ╨┐╨╛╨┤╨│╨╛╤В╨╛╨▓╨╕╤В╤М ╤Б╨╛╨╛╨▒╤Й╨╡╨╜╨╕╨╡ ╨║╨╛╨╝╨╝╨╕╤В╨░ ╨╕ ╨б╨Я╨а╨Ю╨б╨Ш╨в╨м ╨┐╨╛╨┤╤В╨▓╨╡╤А╨╢╨┤╨╡╨╜╨╕╨╡
  commit/push.** ╨Э╨╡ ╨┐╨╛╨┤╨╜╨╕╨╝╨░╤В╤М ╨▓╨╡╤А╤Б╨╕╤О ╨┐╤А╨╕ ╨║╨╛╨╝╨╝╨╕╤В╨╡ ╤З╨╕╤Б╤В╨╛ ╨▒╤Н╨║╨╛╨▓╤Л╤Е ╨╕╨╖╨╝╨╡╨╜╨╡╨╜╨╕╨╣.
- Go тАФ ╤В╨╛╨╗╤М╨║╨╛ permissive-╨╗╨╕╤Ж╨╡╨╜╨╖╨╕╨╕ (MIT/BSD/Apache-2.0).
- `.gitattributes` ╤Б `-text` ╨╜╨░ `manifest.json`/`main.js`/`styles.css` тАФ ╨╡╤Б╤В╤М.
- ╨Я╤А╨░╨▓╨║╨░ ╨┤╨░╨╜╨╜╤Л╤Е ╨▓ ╨С╨Ф ╨╛╨▒╤П╨╖╨░╨╜╨░ ╨┤╨▓╨╕╨│╨░╤В╤М `updated_at`.

## ╨Ш╤Б╤В╨╛╤А╨╕╤П ╤А╨░╨▒╨╛╤В

### 2026-10-26 тАФ v0.1.0 (╤Б╨╛╨╖╨┤╨░╨╜╨╕╨╡)

╨Я╨╛╨╗╨╜╤Л╨╣ ╤Ж╨╕╨║╨╗: brainstorming тЖТ ╨┤╨╕╨╖╨░╨╣╨╜-╤Б╨┐╨╡╨║╨░ тЖТ ╨┐╨╗╨░╨╜ тЖТ ╤А╨╡╨░╨╗╨╕╨╖╨░╤Ж╨╕╤П. ╨а╨╡╤И╨╡╨╜╨╕╤П
╨┐╨╛╨╗╤М╨╖╨╛╨▓╨░╤В╨╡╨╗╤П, ╨╖╨░╤Д╨╕╨║╤Б╨╕╤А╨╛╨▓╨░╨╜╨╜╤Л╨╡ ╨▓ ╤Б╨┐╨╡╨║╨╡: ╨╛╤В╨┤╨╡╨╗╤М╨╜╤Л╨╣ ╨┐╨╗╨░╨│╨╕╨╜ ╤Б╨╛ ╤Б╨▓╨╛╨╕╨╝ ╤Б╨╡╤А╨▓╨╡╤А╨╛╨╝ ╨╕
╤Б╨▓╨╛╨╕╨╝ ╨▒╨╛╤В╨╛╨╝; ╤Д╨╛╤А╨╝╨░ ╨Ъ╨Я ╨║╨╛╨╜╤Д╨╕╨│╤Г╤А╨╕╤А╤Г╨╡╤В╤Б╤П ╨░╨┤╨╝╨╕╨╜╨╕╤Б╤В╤А╨░╤В╨╛╤А╨╛╨╝ ╨┐╨╛ ╨╛╨▒╤А╨░╨╖╤Ж╤Г ╨Ы╨Ш╨Ь╨б; ╨║╨░╤А╤В╨╛╤З╨║╨╕
╤Б╨╕╤Б╤В╨╡╨╝ ╨╕ ╨╝╨░╤В╨╡╤А╨╕╨░╨╗╨╛╨▓ ╨╖╨░╨▓╨╛╨┤╨╕╤В ╨░╨┤╨╝╨╕╨╜╨╕╤Б╤В╤А╨░╤В╨╛╤А; ╨▓╤Е╨╛╨┤ ╨▓ ╨▒╨╛╤В╨░ ╨┐╨╛ e-mail ╨╕ ╨║╨╛╨┤╤Г
(╤Б╨░╨╝╨╛╨╖╨░╨┐╤А╨╛╤Б ╨╛╤В╨║╤А╤Л╤В ╨▓╤Б╨╡╨╝, ╨║╨╛╨┤ ╤В╨░╨║╨╢╨╡ ╨╝╨╛╨╢╨╡╤В ╨▓╤Л╨┤╨░╤В╤М ╨░╨┤╨╝╨╕╨╜╨╕╤Б╤В╤А╨░╤В╨╛╤А); ╤Н╨║╤Б╨┐╨╛╤А╤В
**╤В╨╛╨╗╤М╨║╨╛ DOCX** (PDF ╨╜╨╡ ╨┤╨╡╨╗╨░╨╡╨╝); ╨▒╨╛╤В ╨╛╤В╨┤╨░╤С╤В ╤Д╨░╨╣╨╗ ╨▓ ╤З╨░╤В, ╤Б╤Б╤Л╨╗╨║╨╛╨╣ тАФ ╨┐╨╛ ╨║╨╜╨╛╨┐╨║╨╡;
╤Б╨╛╨│╨╗╨░╤Б╨╛╨▓╨░╨╜╨╕╤П ╨╜╨╡╤В, `draft` тЖТ `final`.

- **`offers-service`**: ╨║╨░╤В╨░╨╗╨╛╨│, ╤Д╨╛╤А╨╝╨░ ╨Ъ╨Я, ╨Ъ╨Я ╤Б╨╛ ╤Б╨╜╨╕╨╝╨║╨╛╨╝ ╤Ж╨╡╨╜ ╨╕ ╨╜╤Г╨╝╨╡╤А╨░╤Ж╨╕╨╡╨╣
  `{NNN}/{yyyy}` (advisory-lock ╨╜╨░ ╨│╨╛╨┤), DOCX ╨╜╨░ ╨║╨╛╤А╨┐╨╛╤А╨░╤В╨╕╨▓╨╜╨╛╨╝ ╨▒╨╗╨░╨╜╨║╨╡ ╤Б
  ╨┐╨╛╨┤╤Б╤В╨░╨╜╨╛╨▓╨║╨╛╨╣ ╨┐╨╗╨╡╨╣╤Б╤Е╨╛╨╗╨┤╨╡╤А╨╛╨▓, ╨║╨╛╨┤╤Л ╨▓╤Е╨╛╨┤╨░ ╨╕ ╤А╨╛╨╗╨╕ ╨▒╨╛╤В╨░, bot API ╨┐╨╛╨┤
  `X-Service-Secret`, Swagger ╤Б ╨┐╨╡╤А╨▓╨╛╨│╨╛ ╨║╨╛╨╝╨╝╨╕╤В╨░.
- **`offers-bot`**: long polling ╤З╨╡╤А╨╡╨╖ SOCKS5, ╨▓╤Е╨╛╨┤ ╨┐╨╛ e-mail+╨║╨╛╨┤╤Г, ╤Б╨▒╨╛╤А╨║╨░ ╨Ъ╨Я
  (╨║╨░╤В╨░╨╗╨╛╨│, ╤Б╨▓╨╛╨▒╨╛╨┤╨╜╤Л╨╡ ╤Б╤В╤А╨╛╨║╨╕ ┬л╨Э╨░╨╖╨▓╨░╨╜╨╕╨╡ | ╨║╨╛╨╗-╨▓╨╛ | ╤Ж╨╡╨╜╨░┬╗, ┬л╨Ь╨╛╨╕ ╨Ъ╨Я┬╗), DOCX ╤Д╨░╨╣╨╗╨╛╨╝
  ╨╕ ╤Б╤Б╤Л╨╗╨║╨╛╨╣.
- **╨Я╨╗╨░╨│╨╕╨╜**: ╤Д╨░╤Б╨░╨┤ ╨╕╨╖ ╨┐╤П╤В╨╕ ╤А╨░╨╖╨┤╨╡╨╗╨╛╨▓, ╤Б╨▓╨╛╨╣ ╨▒╨╗╨╛╨║-╤А╨╡╨┤╨░╨║╤В╨╛╤А, ╨╝╨╛╨┤╨░╨╗╨║╨╕ ╨║╨░╤В╨░╨╗╨╛╨│╨░,
  ╤Б╨┐╤А╨░╨▓╨║╨░, ╨╜╨╛╨▓╨╛╤Б╤В╤М ╨▓ ╨ж╨г╨Я.
- **╨Ш╨╜╤Д╤А╨░╤Б╤В╤А╤Г╨║╤В╤Г╤А╨░**: `sbe-core/docker` тАФ `offers-db`, `offers`, `offers-bot`,
  ╤Б╨░╨╣╨┤╨║╨░╤А `offers-nl-proxy`, Caddy `/api/offers/*` ╨┤╨╛ ╨╛╨▒╤Й╨╡╨│╨╛ `/api/*`, seed
  ╨┐╤А╨╕╨╗╨╛╨╢╨╡╨╜╨╕╤П `offers` ╨▓ `auth-service/seed.go`.
- **╨Я╤А╨╛╨▓╨╡╤А╨║╨╕**: `go test` тАФ 24 ╤В╨╡╤Б╤В╨░ (╤Б╤Г╨╝╨╝╨░ ╨╕ ╨╛╨║╤А╤Г╨│╨╗╨╡╨╜╨╕╨╡ ╨║╨╛╨┐╨╡╨╡╨║, ╨╜╨╛╨╝╨╡╤А,
  ╨▓╨░╨╗╨╕╨┤╨░╤Ж╨╕╤П ╤Д╨╛╤А╨╝╤Л ╨╕ ╨Ъ╨Я, DOCX ╤Б ╤Н╨║╤А╨░╨╜╨╕╤А╨╛╨▓╨░╨╜╨╕╨╡╨╝ ╨╕ ╨╖╨░╤Й╨╕╤В╨╛╨╣ ╨╛╤В ╨┐╤Г╤Б╤В╨╛╨│╨╛ ╨Ъ╨Я, ╨║╨╛╨┤╤Л ╨╕
  ╨╕╤Е ╨╕╤Б╤В╨╡╤З╨╡╨╜╨╕╨╡, ╤А╨╛╨╗╨╕); ╨▒╨╛╤В тАФ 10 ╤В╨╡╤Б╤В╨╛╨▓ (╨╝╨░╤И╨╕╨╜╨░ ╤Б╨╛╤Б╤В╨╛╤П╨╜╨╕╨╣ ╨▓╤Е╨╛╨┤╨░, ╤Б╨▒╨╛╤А╨║╨░ ╨╕ ╨╕╤В╨╛╨│
  ╤З╨╡╤А╨╜╨╛╨▓╨╕╨║╨░, ╨┐╤Г╤Б╤В╨╛╨╣ ╨║╨░╤В╨░╨╗╨╛╨│); `npx tsc --noEmit` ╨╕ `npm run build` тАФ ╤З╨╕╤Б╤В╨╛;
  `check_offers_openapi.mjs` тАФ 31 ╨╝╨░╤А╤И╤А╤Г╤В, ╤Б╤Е╨╡╨╝╨░ ╤Б╤Е╨╛╨┤╨╕╤В╤Б╤П.

## ╨б╤В╨░╤В╨╕╤Б╤В╨╕╨║╨░ ╨╛╤И╨╕╨▒╨╛╨║ ╨╕ ╨╛╤В╤Б╤В╤Г╨┐╨╗╨╡╨╜╨╕╨╣

- ╨Э╨░╤А╤Г╤И╨╡╨╜╨╕╨╣ ╨┐╤А╨░╨▓╨╕╨╗ ╨╜╨╡╤В: 0 `any`, 0 `fetch` ╨▓ ╨┐╨╗╨░╨│╨╕╨╜╨╡, 0 ╨╕╨╜╨╗╨░╨╣╨╜-╤Б╤В╨╕╨╗╨╡╨╣,
  `window.setTimeout()` ╨║╨╛╤А╤А╨╡╨║╤В╨╡╨╜, ╨▓╤Б╨╡ `catch(e: unknown)` + `errorMessage()`.
- `npx tsc --noEmit` EXIT=0, `npm run build` OK.
- ╨Э╨░╨╣╨┤╨╡╨╜╨╛ ╨┐╤А╨╕ ╨┐╤А╨╛╨│╨╛╨╜╨╡ ╤В╨╡╤Б╤В╨╛╨▓ ╨╕ ╨╕╤Б╨┐╤А╨░╨▓╨╗╨╡╨╜╨╛: `roundMoney` ╤В╨╡╤А╤П╨╗ ╨║╨╛╨┐╨╡╨╣╨║╤Г ╨╜╨░
  `1.005` (╨╝╨╜╨╛╨╢╨╕╤В╨╡╨╗╤М 1000 ╨▓╨╝╨╡╤Б╤В╨╛ 100); `headingXML` ╨╛╤В╨┤╨░╨▓╨░╨╗ `Heading1` ╨▓╨╝╨╡╤Б╤В╨╛
  `Heading2` ╨┤╨╗╤П level 3; `validateCatalogItem` ╨╛╤В╨▓╨╡╤А╨│╨░╨╗ ╨║╨░╤А╤В╨╛╤З╨║╤Г ╨▒╨╡╨╖ ╤П╨▓╨╜╨╛╨│╨╛
  ╤В╨╕╨┐╨░, ╤Е╨╛╤В╤П ╨┐╤Г╤Б╤В╨╛╨╣ ╤В╨╕╨┐ ╨┐╨╛ ╤Г╨╝╨╛╨╗╤З╨░╨╜╨╕╤О `system`.

## Выпуск 0.1.0 — 2026-10-27

**Задеплоено на сервер:** `offers-db`, `offers`, `offers-nl-proxy` подняты,
auth-service пересобран (seed приложения `offers`). **`offers-bot` остановлен:**
нужен токен своего бота из @BotFather в `OFFERS_TELEGRAM_BOT_TOKEN` (в `.env`
пусто). После заполнения — `docker compose up -d offers-bot`.

**E2E (зелёный):** `permissions/me` → `superadmin`; каталог (создание, поиск по
слову); форма отдаётся с базовыми полями; КП № 01/2026 с итогом 2999.97
(3 × 999.99 без хвостов); DOCX 77 КБ с `document.xml`, `header1.xml`,
`footer1.xml` бланка и названием клиента со спецсимволами; пустое КП → 400;
без токена → 401.

**Выложено на самораздачу:** `backend` → `main`, обе ветки запушены,
`https://epyur.fvds.ru/plugins/sbe-offers/*` отдаёт те же три sha256, что
локальная сборка; запись `sbe-offers` есть в реестре на сервере и в
`sbe-apstore-registry`.

**Найдено и исправлено при деплое:** `roleRank` не знал `superadmin` (владелец
приложения получал 403); ненастроенная форма отдавалась пустой; `go.mod`
требовал go 1.25 при образе 1.24.

**Скрипты:** `scripts/offers_db_role.sh` (роль `offers_app`),
`scripts/offers_token.sh` (JWT для E2E), `scripts/offers_e2e.sh`,
`scripts/offers_release_check.sh`.

## 0.1.1 — 2026-10-27 (исправление доступа)

Жалоба пользователя: «не могу заполнять каталог, UI выглядит битым».

**Причина.** Роль в центре называется `superadmin` (глобальный администратор), а
плагин проверял ровно `admin`: у владельца приложения не было ни кнопки
«+ Карточка» в каталоге, ни разделов «Форма КП» и «Бот и доступ». Сервер
пропускал (его `roleRank` считает `superadmin` выше `admin` — починено ещё при
деплое), а вьюха молчала.

**Правка.**
- `isAdminRole()` — административный уровень это `admin` **или** `superadmin`;
  `canEditOffers()` — сборка КП для `editor` и выше. Ими пользуются вьюха,
  каталог и список КП вместо строковых сравнений.
- Каркас вьюхи строится до запроса роли: раньше ошибка «Ключ недействителен»
  терялась — подзаголовка, куда её писать, ещё не было.
- В шапке — «Обновить права»: роль можно перечитать без перезапуска.
- При смене роли недоступный раздел сбрасывается на «КП».
- Раздел «Бот и доступ» больше не мигает пустым контейнером при загрузке;
  `superadmin` показывается в списке отдельным пунктом (не выбирается — его
  выдаёт только ЦУП).

**Выпуск:** 0.1.0 → **0.1.1**, самораздача (три sha256 сверены), запись в
реестре на сервере и в git обновлена, текст новости в ЦУП переписан.
`npx tsc --noEmit` EXIT=0, `npm run build` OK.

**Уроки (стоило поймать раньше):** клиент и сервер должны брать одну и ту же
модель ролей из одного места. Серверную часть (`roleRank`) поймал E2E на
сервере, клиентскую — только живая жалоба: локальные тесты и E2E проверяли
`admin`, а в базе лежал `superadmin`. В `src/types/offers.ts` теперь есть
`ROLE_RANK` с `superadmin` — сверять новые проверки ролей по нему.

## 0.1.2 — 2026-10-27 (проверка на локальном стенде, навык ub-mode)

**Как проверяли.** Подняли систему целиком в локальном Docker:
`docker-compose.local.yml` — Postgres 16 + `offers-service` из своего Dockerfile +
заглушка auth-service (`scripts/auth-stub.mjs`, отдаёт карту ролей). Без заглушки
права проверить нельзя: сервис спрашивает их у auth-service.

**Интеграционный прогон** — `scripts/offers_local_e2e.mjs`: **59 проверок по
HTTP** (health, Swagger, авторизация 401/403/просрочка/чужой app_id, каталог
create/update/archive/search/валидация, форма save/read/валидация, предложения —
расчёт, нумерация, права, DOCX, ручки бота под секретом, коды входа,
идемпотентность). Все зелёные. Отдельно проверены сквозной сценарий бота (вход по
коду → сборка → DOCX → «Мои КП» → правка состава) и содержимое готового
`document.xml`.

**Пять дефектов, которых юнит-тесты не видели:**

1. **Сохранённая форма не доходила до DOCX** (главный). Проверка «форма пустая»
   стояла ДО разбора JSON и сравнивала nil-слайс — срабатывала всегда, поэтому
   администратор настраивал форму, а в файле печатался шаблон по умолчанию.
   Теперь «не настроена» = строки нет в БД; пустые массивы — это настроенная
   пустая форма. Тест `TestResolvedFormKeepsSavedContent`.
2. **Сервис не стартовал без S3** (`log.Fatalf`). Теперь работает без хранилища,
   ссылки на файлы честно отвечают «хранилище не настроено».
3. **Экспорт DOCX зависал до 5 минут** при недоступном S3 (rclone ждал таймаут).
   Экспорт байтами больше не обращается к хранилищу: `renderOffer` отделён от
   `buildOfferDocx`.
4. **Правка несуществующей карточки → 500 «db error»** вместо 404 (`pgx.ErrNoRows`).
5. **Имя файла в чате** было `ООО_Из_Бота` — `sanitizeKey` заменял пробелы
   подчёркиваниями; теперь имя читаемое, чистятся только запрещённые символы.

**Правки по навыку ub-mode (первый заход, до локального прогона):**
`isAdminRole()` (superadmin в проверках прав — та же ошибка, что была в плагине),
`dbError()` вместо 30 копий `log.Printf` + `writeJSON`, `prepareOffer()` как
единая граница входа, `src/utils/money.ts` — одно округление денег вместо трёх
копий, удалён мёртвый `formatPrice`, подключён `renderPlaceholderPicker`.
Исправлен **баг денег в боте**: `1.005` печаталось как «1,00» вместо «1,01».

**Покрытие Go:** 16.9% → 23.2% (45 тестов). Ручек HTTP в покрытии пока нет —
они проверяются интеграционным прогоном, пока не подключён Postgres к `go test`.

**Выпуск:** 0.1.1 → **0.1.2**, main запушен, самораздача (три sha256 сверены),
реестр обновлён, новость в ЦУП переписана под этот выпуск.

**Стенд:** `docker compose -f docker-compose.local.yml up -d --build`, затем
`node scripts/offers_local_e2e.mjs`; в репозиторий compose и скрипты входят (они
нужны следующему, кто будет проверять), dist и бинари — нет.

## Серверный код восстановлен в git — 2026-10-27

**Что было.** При выпуске 0.1.2 правки `offers-service/` делались на ветке `main`,
где эти папки закрыты `.gitignore` (коммит `9ffa671` «main — только код плагина»).
Поэтому `git add -A` их не взял: коммит `ef4e987` содержит только плагин, а Go-правки
жили лишь на диске. Ветку `backend` создали от `main` уже ПОСЛЕ удаления папок из
индекса, так что серверный код не попал ни туда, ни туда — `git ls-tree backend` давал
0 файлов в `offers-service/` и `offers-bot/`. Код уцелел на диске и на сервере,
переключение веток его не затёрло.

**Что сделано (на `backend`).**
1. Из `.gitignore` убраны строки `offers-service/` и `offers-bot/`; исключения
   артефактов остались: `.env*`, `*.exe`, `offers-service/*.test`, `offers-service/cover`,
   `offers-bot/dist/`, `**/rclone.conf`.
2. `offers-service/` (36 файлов Go, Swagger, шаблон DOCX) и `offers-bot/` (8 файлов)
   добавлены и закоммичены на `backend`.
3. На `main` `.gitignore` не тронут: там эти папки закрыты намеренно.
4. Проверка: `git ls-tree -r backend --name-only` показывает файлы обеих папок.

**Проверки перед коммитом (ветка `backend`):** `gofmt -l .` — пусто, `go vet ./...` —
чисто, `go test -count=1 ./...` — **45 тестов, покрытие 22.9%**; `offers-bot`:
`tsc --noEmit`, `npm run build`, `node --test` — 10 тестов; плагин: `npx tsc --noEmit`
EXIT=0, `npm run build` OK; `scripts/check_offers_openapi.mjs` — 31 маршрут, схема
совпадает.

**Правило на будущее:** перед коммитом правок сервиса — проверять, что файлы попали
в индекс (`git status --short`), а не просто лежат на диске. Если правка сервиса
нужна, а ветка `main` — переключаться на `backend` ДО правок, не после.

## 0.1.3 — 2026-10-27 (деплой и самораздача)

**Что вошло.** Плагин не менялся по существу: только версия 0.1.2 → **0.1.3** и
переписанный текст новости в ЦУП. Номер поднят потому, что с прошлого выпуска в
`main` появился новый `main.js` (правки 0.1.2 лежали в `backend` и до `main` не
дошли), а повторять номер нельзя — обновление бы не приехало.

**Деплой сервера.** `docker compose up -d --build offers` пересобрал образ, но
контейнер остался старым (`Created` и `StartedAt` не двигались): Caddy проксирует
на него, а сам он жил с 13:49. Понадобилось `docker compose up -d --force-recreate
--no-deps offers`. **Правило: после `--build` проверять, что у контейнера новый
`docker ps` `CREATED`, и только потом закрывать деплой.**

**Проверки на бою.** health 200, `/docs/` 200, `/openapi.yaml` 200, без токена 401;
`permissions/me` → `superadmin`; каталог (4 карточки), форма (`client`,
«Коммерческое предложение»), список КП (№ 001/2026) читаются; в БД шесть таблиц,
данные на месте.

**Не задеплоено по причине, которую нельзя закрыть кодом.** `offers-bot` остаётся
остановленным: `OFFERS_TELEGRAM_BOT_TOKEN` в `.env` пуст, нужен токен своего бота
из @BotFather. После заполнения — `docker compose up -d offers-bot`.

**Самораздача.** `lims_selfhost.sh sbe-offers` — три sha256 сошлись на сервере и
по https: `manifest e1796cf…`, `main 2836063…`, `styles 3092269…` (styles не
менялся с 0.1.1). `POST`-строка `registry_file_overrides` обновлена; git-копия
`sbe-apstore-registry/registry.json` — коммит `46811dc`.

**`registry.json` на диске сервера — не источник выдачи.** Caddy отдаёт
`/registry.json` не файлом, а проксирует в auth-service, который накладывает
сверху `registry_file_overrides` из БД. Поэтому правка файла в `/opt/mailers/www/`
ни на что не влияет; проверять выдачу — только по https или по таблице.

## 0.1.4 - 2026-10-29 (bot token is entered from the plugin)

User request: "it is not clear how to enter or change the bot token in order to
start the bot". Option A was chosen by the user (own setting in offers-service).

**Cause.** The token came only from `TELEGRAM_BOT_TOKEN` in the bot's environment,
i.e. `OFFERS_TELEGRAM_BOT_TOKEN` in the server `.env`: setting it needed SSH, a
change needed a container restart, and an empty token made the poller return silently
while the "Bot and access" section looked healthy.

**What was done.**
- `offers-service`: table `offers_bot_config` (single row: token, bot username,
  author, `bot_last_seen_at`, `bot_last_error`); `botconfig.go` and six routes;
  the plugin's ones behind `requirePerm("admin")` - `GET/POST/DELETE /bot/config`
  and `GET /bot/status`; internal ones behind `requireServiceSecret` -
  `GET /bot/token` and `POST /bot/heartbeat`. The token is never returned to the
  plugin.
- Saving verifies the token first (`getMe` through the bot) and writes only on
  success, so a typo cannot replace a working token. Bot unreachable -> the request
  fails and nothing is stored.
- `offers-bot`: `resolveBotToken()` reads `GET /api/offers/bot/token`, caches
  for 15 s and **throws** on any failure - no fallback to the environment, the lesson
  from the `portal-embed` incident of the same day. The poller no longer exits
  without a token: it heartbeats the reason and retries every 15 s, so the token
  entered in the plugin takes effect without a restart. New `verify-server.ts`
  serves `POST /internal/verify` on the internal network only.
- Plugin: a "Telegram-бот" card at the top of "Bot and access" - masked field,
  "Проверить и сохранить", "Удалить токен", "Проверить сейчас", a coloured state
  line ("Бот работает: @..." / "Токен не задан" / "Бот не отвечает · ошибка") and
  the @BotFather hint.
- `TELEGRAM_BOT_TOKEN` removed from `docker-compose.yml` and both
  `.env.example` files; `offers` got `OFFERS_BOT_URL`, `offers-bot` got
  `VERIFY_PORT`.

**Checks.** `gofmt`/`go vet` clean, `go test` - 51 tests, coverage 23.1%
(6 new: token normalisation, format check, no token field in the response, verify
via bot: ok / Telegram error / bot down). Bot: typecheck, build, 19 tests (9 new,
including "does not fall back to `TELEGRAM_BOT_TOKEN`"). Plugin: `tsc --noEmit`
EXIT=0, `npm run build` OK. Swagger: 37 routes, schema matches
(`check_offers_openapi.mjs`). Local stand: `bot-stub.mjs` plus a new section 10
in `offers_local_e2e.mjs` - 77 checks green, twice in a row.

**Not done yet.** No commit, no deployment, no self-hosting: the server still runs
the old bot with the token from `.env`. Deploying needs
`docker compose up -d --build offers offers-bot` and then entering the token in the
plugin; the `OFFERS_TELEGRAM_BOT_TOKEN` line should be removed from the server
`.env` (backup first).

**ub-mode checks (2026-10-29).** ``dryer`` finds no duplication in the new code
(the remaining pairs are pre-existing). ``crapper`` on the fresh code flagged four
oversized functions, all split before releasing: the bot card (CC 13 -> pieces of 5
and below), the access editor (10/12 -> 1 and 6), ``verifyTokenViaBot`` (9 -> 5 plus
``askBotToVerify`` 6) and ``startVerifyServer`` (10 -> 3 plus ``handleVerify`` 8).
Recommended thresholds are CRAP < 10 and duplication < 0.8; the remaining high CRAP
values come from zero test coverage on UI and HTTP, the same as everywhere else in
this plugin, and are recorded rather than hidden.

## Roles left the bot, sign-in survives a restart — 2026-10-29 (commit fbf036b)

**User's requirement:** plugin roles and bot roles must not overlap at all; the bot
needs no roles — anyone who gets access to the bot may build offers. Two-level roles
had been matched obscurely, so "Менеджер" in the plugin showed up as "просмотр" in the
bot.

**Done.**
- `offers-bot/src/session.ts`: the `role` field is gone from `ChatSession`;
  `applyLogin(session, email)` takes no role.
- `offers-bot/src/flow.ts`: `AUTH_OK(email)` no longer names a role,
  `NOT_ALLOWED_TO_EDIT` deleted.
- `offers-bot/src/poller.ts`: the gate in `startDraft` removed.
- `offers-bot/src/api.ts`: `BotIdentity { email }` replaces `{ email, role }`, so the
  compiler prevents leaning on a role again.
- `offers-service/access.go`: `bot/identity` and `bot/verify-code` return only `email`;
  `roleForEmail` deleted as dead code.
- Plugin `src/ui/access-editor.ts`: the "Роль" column and the role picker are gone;
  `roleLabel` deleted. **Plugin roles (viewer/editor/admin) stay in the plugin only.**

**Sign-in survives a restart.** The user reported being asked for a code after every
bot restart. The permanent `chat_id → email` link was already in `offers_bot_link` and
never lost; what was lost was the in-memory session. New `restoreSession(email)` in
`session.ts` (pure, tested) and `sessionRestored(chatId)` in the poller: for a chat
absent from memory the bot asks `identity`, so a known chat is immediately signed in.
Used in both the text handler and the callback handler. An unknown chat still gets a
code; an unreachable service does not break sign-in.

**Checks.** TDD: tests written first and seen failing for the right reason
(`roles-removed.test.ts` 3 checks, `restore.test.ts` 3). Bot: 28 tests, `tsc` and
`tsc --noUnusedLocals` clean, build OK. `crapper`: `sessionRestored` CC 5,
`restoreSession` CC 2 — within the threshold; `dryer` finds no duplication in the new
code. Dead import `draftTotal` removed.

**Deployed.** Bot container recreated; a known chat is recognised without a code
(`810181470 → polishchuk@tn.ru`, `8905120569 → miloserdov@tn.ru`), an unknown chat gets
404 from `identity`, which means a code will be requested.

### Debt: dispatcher functions (deliberately not refactored)

`handleCallback` (CC 24) and `handleText` (CC 23) in `offers-bot/src/poller.ts` stay as
they are — the user chose to record them as debt rather than refactor now. Why they
are over the CRAP threshold:

- both are dispatchers mixing three concerns: anti-flood/entry guard, sign-in state
  machine, and a table of handlers;
- sign-in is checked twice (once per function) — the restart fix above had to be
  written in both places;
- `back` and `done` each repeat the "draft not found" branch with slightly different
  text; `pick:` and `link:` carry their own `try/catch`;
- `done` builds a one-element `missing` array instead of `if (!draft.clientName)`;
- nothing is testable: checking "undo removes the last item" requires the whole poller
  with network.

Planned shape when taken on: `handleText` → guard → ensureAuthed → command table;
`handleCallback` → answer → ensureAuthed → prefix table; the ten buttons become small
functions in `callbacks.ts`, texts in `flow.ts`. Order: tests on current behaviour
first, then the move, then `crapper`/`dryer`.

## Brief dialog, skill-driven copy, KB system cards - 2026-10-29 (structural cycle)

User request: from "the bot builds a list of positions" to a **brainstorming
conversation** about the object, then model-written section copy agreed with the
manager, with the four marketing skills adapted to a text document (not slides).
Then the customer card moved into the Knowledge Base instead of a form field.

**Architecture added**

- `offers-service/model/` - domain types (`Offer`, `BriefPayload`,
  `ProductContext`). A separate package because `main` cannot be imported:
  prompts and checklists could not be tested otherwise.
- `offers-service/skills/` - writing rules adapted from
  `coreyhaines31/marketingskills` (MIT): `sales-enablement` 2.4.0,
  `copywriting` 2.3.2, `copy-editing` 2.2.1, `product-marketing` 2.2.0.
  Deviations recorded in `skills/README.md`. Own additions:
  `CheckClaimNumbers` (model may only use numbers that exist in the offer) and
  `[НУЖНО: ...]` instead of invented facts.
- `offers-service/skills/systemcard.go` - parses a KB note into a system card.
- `offers-service/kbclient.go` - KB client; inter-service JWT from
  `auth-service` (`/internal/service-token`, `target_app_id=kb`), same path as
  `lab-service` -> `ekn-service`.
- Bot dispatchers split: `routes.ts` + `callbacks.ts`; `poller.ts` is now
  transport only (debt from the previous cycle closed).

**Three defects the tests found before the rules were written**

1. `\b` in Go does not treat Cyrillic as word characters: `\bне\s+...` silently
   matched nothing. Both AI-tell regexes were rewritten without `\b`.
2. `\u00a0` is not a valid Go regexp escape - panic at package init.
3. JSON answer of a "skipped" topic is indistinguishable from "model forgot the
   field": both leave the value empty. Added explicit `skipped_topics` in the
   brief and `skip` in the model answer.

**DOCX was "broken" - and the template was not the cause.** `standard.docx` in
`sbe-offers` and `sbe-lims` is byte-identical (sha256 `1FE4F368...`).
`buildDocx` was replacing `word/styles.xml` (5 styles instead of 58) and
`word/numbering.xml` (2 lists instead of 12). Page numbers in the footer are
`PAGE`/`SECTIONPAGES` fields inside text boxes and depend on brand styles - they
were pushed out. Fixed the same way as LIMS: both parts taken from the template
untouched, our markup moved to brand styles (`Normal` = `a0`, `heading 1` = `1`,
bullet list = `numId 6`). A second, cheaper-to-fix-now issue: `wp:docPr` ids are
computed across all package parts (header/footer take 1..25) instead of a
constant `1000`; media written as `offerimgN.*` so `image1/image2` of the brand
are not overwritten.

**Customer context lives in the Knowledge Base.** A note tagged
`commercial_propose` with `system_sku`, `## Состав системы`,
`## Описание системы`, `## Свойства и преимущества`. Standard:
`docs/sbe-offers-system-card-standard.md`; template note
`_Шаблон карточки системы.md` in the systems folder. `offers_context` became a
cache (mode `system_cards`), not a place to type text. A KB failure does not
clear the cache: previous cards are returned with `error`.

**Checks.** Go coverage 23.4% -> 38.1%, `go test ./...` green in three packages
(`main`, `model` 71.6%, `skills` 89.8%), plugin `tsc --noEmit` EXIT=0, local
stand E2E 77/77, `check_offers_openapi.mjs` 42 routes and schema match.

**Committed** (branch `backend`, no push): `ed7486b` llm client, `e640205`
brief + skills + copy, `54fb992` DOCX fix + KB cards, `8765790` plugin
"Контекст КП". In `sbe-core`: `78352e8` (LLM wiring), `66263ec` (KB wiring) -
only own lines were staged there, other uncommitted work left untouched.

**Open.** Task 9 (bot states for brief/copy), 10 (plugin "Бриф" section), 11
(form blocks, version 0.2.0, docs). Version not bumped yet: the plugin code
changed, so `0.1.5 -> 0.2.0` belongs to the release task, with the ЦУП news text
rewritten for this release.

## Card text in DOCX, file attachments in the bot - 2026-10-30

Closes the three open items of the previous cycle. Committed (branch `backend`,
no push): `f847c47`, `7e23bf1`, and the attachment round.

**Task 7d - card text and photo reach the document.** `renderCardSections`
builds its image collector with `newTemplateMedia()`, not `&docxMedia{}`: the
bare struct has no used-id map and panics with "assignment to entry in nil map"
on the first picture, because the brand template already occupies ids 1..25 in
its headers and footers. A manager-approved section text still wins over the
card: the card only fills what nobody wrote by hand.

**Task 7e - `skills/attachments.go`: reading files from the chat.** The type is
taken from the signature, never from the name: a zip with `xl/workbook.xml` is
xlsx, one with `word/document.xml` is docx, an OLE header is the old `.xls`
(explicit refusal: "resave as .xlsx"), `%PDF` is pdf, anything without control
bytes is markdown. `.md` is laid out by brief keys - "Ключ: значение" is a
field, the list right under it belongs to that field, everything else becomes
notes; an unknown field keeps its label and lands in notes instead of being
dropped. `.xlsx` reads `sharedStrings.xml` plus `sheetN.xml` (both cell kinds),
looks for a header row within the first 20 rows, and recognises a specification
by the "наименование" + "количество" columns; `сумма ≠ количество × цена`
becomes a warning rather than silent correction. Covered by 10 tests,
`skills` coverage 87%.

**Task 7f - two-step file upload.** `POST .../attach` parses and shows "what I
understood", `POST .../attach/apply` writes only what the manager confirmed.
The split exists because a customer's file is a draft: silently applying its
prices would put stale numbers in the offer. Applying a specification takes
prices from the CATALOG for rows matched by name; the file's price is used only
for positions the catalog does not have. Applying a brief merges - it never
overwrites what the manager said in the chat, lists are joined without
duplicates, and an objection without an answer stays out of the brief. The bot
gets raw bytes with the name in `file_name` (no multipart parsing on either
side), 10 MB cap answers 413, ownership is checked like every other bot route.
Bot side: `attach.ts` shows the preview with confirm buttons, `poller.ts`
downloads the document through the same SOCKS proxy as `getUpdates`.

**Checks.** `gofmt`/`go vet` clean, `go test ./...` green (main 38.6%, model
71.6%, skills 87.1%), bot `tsc --noEmit` + build OK and 44 tests (9 new),
`check_offers_openapi.mjs` 44 routes and schema match. Live HTTP run of the
attachment routes was NOT done: Docker Hub was unreachable (TLS timeout on
`auth.docker.io`), so the local stand could not be rebuilt; the plan's section
11 for `offers_local_e2e.mjs` is still to be written.

**Lesson.** Rewriting a Go file through a PowerShell pipeline
(`Get-Content -Raw | Set-Content -Encoding UTF8`) destroyed `attachments.go`:
the file came back as CP1251 mojibake and had to be rewritten from scratch.
Source files are edited with the editor tools only, never re-encoded by shell
text commands.
## Task 9 closed, Task 10-11 done, version 0.2.0 prepared - 2026-10-30 (evening)

Continued the brief-dialog plan (`docs/superpowers/plans/2026-10-29-sbe-offers-brief-dialog-plan.md`).

**Task 9 (bot brief/copy states) was already written but untested live.** Fixed on the way:
- UTF-8 BOM stripped from `callbacks.ts`, `routes.ts`, `flow/writer.ts` (the project's
  known gotcha; a BOM breaks file reading).
- Duplication removed in `flow/writer.ts`: four copies of `numberedSlots(systemsIn(...))`
  became one `activeSlot()` / `slotsFor()`.
- Dead code deleted in `flow/brief.ts`: `briefTopicQuestion`, `canSkipBrief`, `skipBrief`
  were only ever called from their own tests. Those tests now check real behaviour
  (keyboards, summary) instead.
- The plan named `draft.step` values `brief`/`text`; the implementation keeps the single
  `authed` step and tracks the conversation in session fields (`briefTopic`, `copySlot`,
  `copyEditing`). Fewer states, no parallel enum - deliberately better, recorded here.

**Three live defects found by the local stand, all fixed test-first where practical:**

1. `handleBotBriefStep` read `chat_id` from the query string while the bot sends it in
   the JSON body -> 400 `chat_id is required`. Switched to `botOfferForChatQ` (the copy
   route already used it).
2. **Early reject did not drain the request body.** `POST .../attach` answers "not your
   offer" *before* reading the file; Go then never reads the body, so the client kept
   uploading megabytes into an answered request and hung until timeout - a 1 MB file never
   went through. Added `drainBody()` in a `defer` plus `TestAttachEarlyRejectDrainsBody`
   (failed with "4 194 304 bytes left in the body", now green).
3. **One wrong-typed field blanked the whole brief.** The model returns `"why_us": ""`
   instead of `[]`; strict `json.Unmarshal` failed and `mergeBrief` silently returned the
   previous (empty) payload - the manager saw "the answer was not recorded" although every
   other field had arrived. Added `tolerantBriefPayload`: parse field by field, a string
   where a list is expected becomes a one-item list, garbage in one field no longer takes
   the rest. Tests: `TestStepBriefToleratesWrongTypedField`, `TestTolerantBriefPayload`,
   `TestTolerantBriefPayloadGarbage`.

**Task 10 - brief in the plugin.** `GET`/`PUT /api/offers/offers/{id}/brief` (viewer+ for
read; owner or admin for write, role `editor` alone does not edit someone else's brief).
`brief-editor.ts` renders a card inside the offer editor: read mode with honest
"не заполнено" labels, edit mode with `✏️ Править бриф`, and a collapsible turn history.
Pure rules live in `brief-model.ts` (field order mirrors `skills.BriefTopics`, an objection
counts as handled only with an answer). `emptyBriefPayload()` in `types/offers.ts` keeps
the key set in sync with the server.

**Task 11 - document toggles, version, docs.** `doc.brief_summary` and `doc.section_copy`
in `form_values` control the optional DOCX content, **both off by default** so existing
offers print exactly as before - three existing tests had to set the flag explicitly, and
three new tests pin the off state (`TestDocxSectionCopyOffByDefault`,
`TestDocxBriefSummaryToggle`, `TestFormFlagOnNil`). `renderBriefSummary` prints only
non-empty `brief.*` fields. Version `0.1.5 -> 0.2.0`, ЦУП news text rewritten for this
release (it is baked into `main.js` at build time - the 2026-09-17 lesson).

**Local stand.** New `scripts/llm-stub.mjs` (answers `/internal/llm/complete`; the brief
prompt contains "JSON" so it returns `{payload,...}`, otherwise plain section text;
the marker `STUB_FAIL` in the user prompt forces 502 so degradation is exercised on the
real code path) plus an `llm-stub` service in `docker-compose.local.yml`. E2E sections 13
(brief, copy, degradation, foreign chat) and 14 (plugin brief routes, toggles) added.

**Checks.** Go: `gofmt`/`go vet` clean, `go test ./...` green, coverage `main` 39.2%
(model 71.6%, skills 87.1%). Bot: `tsc --noEmit --noUnusedLocals` clean, 62 tests.
Plugin: `tsc --noEmit` EXIT=0, `npm run build` OK at 0.2.0. OpenAPI: 47 routes, schema
matches. Local stand E2E: **131/131 green, twice in a row**.

**ub-mode.** `crapper` on `offers-bot/src`: every function below CC 10
(`routeText` 8, `handleCopyText` 6, `acceptAndAdvance` 5, `handleBriefText` 6).
`dryer` finds one pair at 0.84 in `api.ts` (`catalog`/`listOffers`) - pre-existing, not
from this cycle; folding it would only hide the API shape. Functions still at CRAP > 10
are UI and HTTP handlers with zero coverage, same as everywhere else in this plugin -
recorded rather than hidden.

**Environment note.** Docker Hub was unreachable (TLS timeout on `auth.docker.io`), so the
service image cannot be built from its Dockerfile. Built the binary in the local
`golang:1.23` image (`GOTOOLCHAIN=auto`, `go.mod` asks for 1.24) and layered it over the
existing `sbe-offers-offers:latest`. Host port forwarding to `18080` wedges after aborted
large uploads; the E2E is therefore run inside the `sbe-offers_offersnet` network
(`offers_local_e2e.mjs http://offers-local-app:3000`).

**Not done.** No commit, no push, no deploy, no self-hosting - waiting for an explicit
command.

### Release 0.2.0 - 2026-10-30

**Committed and pushed.** Commit `f7a056f` on `backend`: Task 9 state machine in
the bot, three live defect fixes (`brief/step` body `chat_id`, `drainBody` on early
reject, tolerant brief payload), Task 10 brief routes + plugin card, Task 11 document
toggles. `main.js` rebuilt at 0.2.0 and included, as the store downloads it from `main`.

**`backend` -> `main`.** Per the release rule the plugin part was carried over to `main`
and the suffix dropped; the server folders (`offers-service/`, `offers-bot/`) stay only
on `backend`, they are `.gitignore`d on `main`. Both branches pushed.

**Deployed.** `docker compose up -d --build offers` then verified the container's
`CREATED` actually moved (the 0.1.3 lesson) - a rebuild alone can leave the old container
in place behind Caddy.

**Self-hosting.** Three SHA-256 computed as the last action, after the branch switch and
the build; `scripts/lims_selfhost.sh` uploaded them and updated
`registry_file_overrides`; the three sums were compared against
`https://epyur.fvds.ru/plugins/sbe-offers/*`. The `sbe-apstore-registry` git copy of
`registry.json` was updated to match.

**Recording rule learned here.** The `AGENTS.md` entry was written before the commit, so
the commit hash had to be filled in afterwards; the file then differs from the commit it
describes. Filling the hash in a second, immediately following commit keeps the record
honest - or write the entry after committing and commit it separately.
## "База знаний не настроена" while it is configured - 2026-10-30

User report: the plugin's «Контекст КП» says «База знаний не настроена», although
the Knowledge Base is set up.

**The KB was fine; the container was not wired to it.** In the server
`docker-compose.yml` the `offers` service block had no `KB_SERVICE_URL`,
`KB_SERVICE_SECRET`, `LLM_SERVICE_URL` or `LLM_SERVICE_SECRET`, although the secrets
are present in `.env` and both `kb` and `llm` live on the same `internal` network.
So `NewKBClient()` got an empty base URL, `main.go` logged «KB_SERVICE_URL не задан»,
`s.kb` stayed nil, `GET /context/cards` answered `kbEnabled: false`, and
`context-editor.ts` showed the «не настроена» text. The same missing `LLM_*` pair
means the brief and section copy degrade on the server as well.

**Why the stand did not catch it (my miss).** The plan's Task 7 step 1 says "KB
config in compose", but I never added KB to `docker-compose.local.yml`; every E2E run
up to now had the Knowledge Base absent, so nothing checked that cards reach the
plugin. The message looked like a normal "not configured" state.

**Fixed in the stand first, then on the server.**
- `scripts/kb-stub.mjs` - serves `tags`, `notes`, `notes/{id}`,
  `notes/{id}/attachments`, `file-link`, requires a Bearer token like the real
  service. Two system cards in the standard markup.
- `scripts/auth-stub.mjs` gained `POST /internal/service-token`: without it the
  inter-service JWT is missing and cards cannot be read even with the URL set.
- `docker-compose.local.yml`: `kb-stub` service plus `KB_SERVICE_URL` /
  `KB_SERVICE_SECRET` on `offers`.
- `offers_local_e2e.mjs` section 15: `kbEnabled === true` with a hint naming both
  variables, then `POST /context/refresh` and the card contents. Verified red without
  KB (exactly 2 failures, both naming the cause) and green with it.

**Two more defects found while writing that section:**
1. It checked `GET /context/cards` before `refresh`, but that route serves the
   **cache**, which is empty on a fresh database - the check was wrong, not the code.
   Order fixed: refresh first, then read.
2. The stub card put the description before the first `##`; the parser reads it under
   `## Описание системы`. The stub was wrong, not the parser - fixed to follow
   `docs/sbe-offers-system-card-standard.md`.

**Result.** Stand E2E 140/140 green. Server `docker-compose.yml` updated (backup
`docker-compose.yml.bak-kb-20261030`), `offers` recreated, `kbEnabled` and the brief
checked live.

**Third defect, found only on the live server: the list field name.** After the wiring
fix `kbEnabled` became `true` but `count` stayed `0`. `kb-service` returns the list in
**`items`**, while `KBClient.Notes` read **`notes`** - and my unit test fed it a
made-up `notes` body, so the test agreed with the bug. `Notes` now reads `items` and
falls back to `notes`; `kb-stub` serves `items` like the real service; two tests pin
both shapes (`TestKBClientNotesReadsItemsField`, `TestKBClientNotesReadsLegacyNotesField`).
Live check after redeploy: 2 cards, both with description and composition.

**Lesson that is worth more than the fix:** a stub I write myself is a claim about
someone else's contract. The card list was "checked" in tests that encoded my guess,
not the service's answer. Contracts with foreign services are pinned by a recorded
real response (see `testdata/` in `skills/`), not by an invented fixture.

**Rule for the next service on central permissions.** A plugin-service that talks to
another service needs BOTH values in its compose block - the URL and the secret - and
the stand needs a stub for that service. An empty URL is not an error: the service
starts, reports "not configured" and looks healthy. Add the stub and an E2E check in
the same commit as the client code, not later.
## Materials and systems: cycle started - 2026-10-30 (evening, release 0.2.1)

User requirement: **two separate entities.** A **system** is a combination of
materials chosen for the best result and has **no price of its own**; it exists
to justify *why this combination*. A **material** is what is sold: its own name
without a `ТН-` prefix, its own SKU, and several variants whose number is **ЕКН**
(one SKU, many ЕКН). Prices never live in the catalogue: the manager uploads an
Excel sheet with ЕКН, names and prices, and that price is the truth for this
offer, because the same material costs differently by delivery terms.

Design: `docs/superpowers/specs/2026-10-30-sbe-offers-materials-systems-design.md`
Plan: `docs/superpowers/plans/2026-10-30-sbe-offers-materials-systems-plan.md`

**Live data checked before designing:** the Knowledge Base has **587** cards
tagged `commercial_propose` and **421** tagged `material` (tag ids `11181`,
`11507`). Materials carry both tags; systems only `commercial_propose`. That is
the discriminator the whole split rests on, so it was measured, not assumed.

### Released in 0.2.1 (three live defects)

1. **401 "ключ недействителен"** while the key worked. The plugin never
   re-requested the token, and the service logged no rejections at all - only
   `listening` - so the complaint could not be investigated after the fact.
   Now: first 401 drops the token cache and retries once; a second 401 says
   «Сеанс входа устарел»; 403 says «Нет прав». Every rejection logs
   `auth reject: {method} {path} {cause}` - path and one of `no-token`,
   `bad-token`, `issuer`, `audience`, `app-id`, `role`, never the token or the
   body. The six copies of the gate became one `authorize` (84.2% covered).
   `sbe-core` gained `forgetToken` (additive), `sbe-apstore` exposes it on the
   bridge.
2. **Only 50 cards of 588.** `kb-service` returns at most **50 notes per
   request at any `size`** (measured: 50/200/500/1000 all give 50) while
   reporting `total`. The client asked for 200, got 50 and treated it as the
   whole list - there was no pagination at all. Now it pages until a short page
   or `total`, capped at `maxKBPages = 40`; hitting the cap logs a line instead
   of staying silent.
3. **"Открыть заметку" said the note was not found.** `kb-service` returns the
   **logical** path inside the Knowledge Base
   (`Инженерная изоляция/Системы/ТН-СТИЛОБАТ ЭКСПЕРТ ТРОТУАР.md`), while the
   file lives elsewhere in the vault (`База службы/Write it/…` - checked on
   disk). The plugin now resolves the file by name with normalisation (case,
   ё/е, quotes, spaces); several equal names means it does not guess. The error
   text names the note instead of the misleading path.
   Check: `scripts/check_note_path.mjs` - 14/14 (there is no test runner in the
   plugin; the check lives as a script, same as `check_*_openapi.mjs`).

### Done in code, not yet released (Task 2 of the cycle)

`offers_material` (automatic SKU `MAT-{TYPE}-{SERIES}-{NNNN}`, price **range**
`price_min`/`price_max`, **no price column**) and `offers_material_ekn` (many
versions per material, unique on `(material_id, ekn)`). The migration moves
`kind='material'` rows from `offers_catalog` and archives the system rows; it is
idempotent (three restarts produced no duplicates and no changed SKUs).

A test caught a real defect here: SKU cut `МЕМБ` down to `МЕМ` - type codes are
now four characters, series three.

### Where the cycle stands

Done: Task 1 (401), Task 2 (schema), Task 3b (both user reports above).
Next, in plan order: Task 3 (material/ЕКН routes), 4 (Excel parsing and
matching), 5 (two-step sheet upload), 6 (systems by tags and SKU links),
7 (plugin UI), 8 (bot), 9 (docs, 0.3.0, deploy).

**Not deployed:** the three 0.2.1 fixes exist only in git. The live server still
runs 0.2.0.
## Остановка цикла 2026-10-31: где стоим

Работа остановлена по просьбе пользователя. Состояние на момент остановки.

### Выпущено в 0.2.1 (три живых дефекта) - развёрнуто 2026-10-31

1. **401 «ключ недействителен»** при рабочем ключе. Плагин не перезапрашивал
   токен, а сервис не писал отказов в журнал вовсе. Теперь: первый 401 сбрасы-
   вает кэш и повторяет запрос один раз; второй говорит «Сеанс входа устарел»;
   403 говорит «Нет прав». Отказы пишутся как
   `auth reject: {method} {path} {cause}` - путь и причина, без токена и тела.
   В `sbe-core` добавлен `forgetToken` (аддитивно), в `sbe-apstore` - проброс
   в мост.
2. **«Контекст КП» видел 50 карточек из 588.** `kb-service` отдаёт не больше
   50 заметок на запрос при любом `size` (замерено: 50/200/500/1000 - всюду
   50), но сообщает `total`. Пагинации не было. Теперь листает до неполной
   страницы или `total`, предел `maxKBPages = 40`.
3. **«Открыть заметку» отвечала «не найдена».** Путь из Базы знаний -
   логический (`Инженерная гидроизоляция/Системы/...`), а файл в вольте лежит
   в `База службы/Write it/...` (проверено на диске). Теперь файл ищется по
   названию с нормализацией; несколько одинаковых названий - не угадываем.
   Проверка: `scripts/check_note_path.mjs`, 14/14.

### Сделано в цикле «две сущности» (Tasks 2-5, 6 частично)

- **Task 2.** `offers_material` (авто-SKU `MAT-{ТИП}-{СЕРИЯ}-{NNNN}`, вилка
  цен, **колонки цены нет**) и `offers_material_ekn` (много ЕКН на материал).
  Миграция переносит материалы из каталога и архивирует системы; идемпотентна
  (проверено тремя перезапусками). Тест поймал: SKU обрезал `МЕМБ` до `МЕМ` -
  тип теперь 4 знака, серия 3.
- **Task 3.** Шесть ручек материалов и ЕКН. SKU назначает только сервер;
  `kind=system` отклоняется; правка не меняет SKU; удаление уводит в архив;
  повторный ЕКН не плодит дублей; вилка расширяется, но не сужается.
- **Task 4.** Разбор таблицы (колонки по заголовку, русские числа «1 500,50»,
  ошибка называет недостающую колонку) и сопоставление: точный ЕКН -> точное
  название -> модель только для остатка и только как предложение
  (`needsConfirm`). Модель спрашивают только про неразобранное (проверено
  счётчиком вызовов); «нет» и мусор материал не выдумывают.
- **Task 5.** Двухшаговая загрузка: превью ничего не пишет и называет
  человеку догадки модели, несопоставленные строки, расхождения сумм и
  нулевые цены. Применение пишет цену **из таблицы**, дописывает ЕКН,
  расширяет вилку.
- **Task 6 (незакончена).** Готовы чистые правила: разделение по тегам
  (`commercial_propose`+`material` = материал, иначе система) и извлечение SKU
  из строки состава. Тест снова поймал кириллическую грабле `\b` - код
  `РАБ-...` не находился. **Не сделано:** ручка `GET /api/offers/systems`,
  разрешение состава против материалов и отчёт о проблемах, UI систем.

### Что осталось по плану

| # | Задача | Состояние |
|---|---|---|
| 6 | Системы: ручка, разрешение состава, проблемы | правила готовы, ручки нет |
| 7 | Плагин: «Материалы», «Системы», загрузка таблицы, «Добавить систему» | не начато |
| 8 | Бот: каталог только материалы, разворот системы | не начато |
| 9 | Документация, 0.3.0, деплой, самораздача | не начато |

### Коммиты цикла (backend)

`402662a` 401 и журнал отказов, `c03ae69` материалы и ЕКН (схема),
`4fc2a69` пагинация карточек, `5d61782` поиск заметки по названию,
`e8b0773` 0.2.1b и запись о старте цикла, `97c6f24` ручки материалов,
`805a428` разбор таблицы и сопоставление, `b7c7500` загрузка двумя шагами,
`2f2d3c8` системы и материалы по тегам.

В соседних репозиториях: `sbe-core` `09a3cb2` (`forgetToken`),
`sbe-apstore` `f74a6dd` (проброс в мост).

### Проверки на момент остановки

`go test` зелёный (покрытие `main` 40.0%, `model` 71.6%, `skills` 84.0%),
`tsc --noEmit` EXIT=0, `npm run build` OK, OpenAPI 55 маршрутов совпадают,
стенд E2E 156/156, `check_note_path.mjs` 14/14.

**Развёрнуто 2026-10-31.** Сервис и бот пересобраны и пересозданы, самораздача выложена и сверена по https. Проверено на бою: context/refresh собрал **588 карточек** (было 50), kbEnabled: true, журнал отказов пишется, таблицы offers_material/offers_material_ekn созданы миграцией, 1 материал перенесён из каталога, 3 системных карточки заархивированы.
### Грабли, стоившие работы в этом цикле

Два места, о которые легко споткнуться снова. Записаны сюда, потому что в
прошлый раз они уже подводили, а знание осталось только в комментарии кода.

**1. `\b` в Go не считает кириллицу словом.** Регулярка `\b(?:MAT|РАБ)-\d{4}\b`
не находит `РАБ-МОНТ-ОБЩ-0001`: граница слова в Go определяется по
ASCII-буквам, и кириллическая `Р` не считается «словом» с левой стороны.
Шаблон молча не срабатывает — ошибки нет, просто пустой результат.

- Поймано дважды: 2026-10-29 в `skills` (выражения «ИИ-штампов» не ловили
  ничего) и 2026-10-31 в `matcher.go` (`extractCompositionSku`).
- **Правило:** в регулярках по русскому тексту `\b` не использовать. Границу
  задавать явно — классом символов, `(?:^|[^А-ЯЁA-Z0-9])` или проверкой
  результата. Тест на кириллицу обязателен: одна строка `РАБ-…` ловит всё.

**2. `crapper` показывает 0% покрытия на Go-функциях с табличными тестами.**
Инструмент определяет вызовы эвристикой по тексту и не узнаёт вызовы внутри
`cases := map[...]` с `t.Run`. На функциях этого цикла он печатал `0.0%` и
CRAP 100–300, хотя реальное покрытие 96–100%.

- **Правило:** покрытие Go смотреть через `go tool cover -func`, а не через
  `crapper`. `crapper` годится только для цикломатической сложности (CC) —
  и то как ориентир: CC у `buildPreview` 17 и `matchRows` 16 при 100%
  покрытии, то есть порог CRAP < 10 здесь не нарушен, а перебор веток
  осмысленный (это чистые диспетчеры правил).
- Команда для проверки покрытия по одной функции:

```bash
go test -coverprofile=/tmp/c.out ./... && go tool cover -func=/tmp/c.out | grep -E 'функция|total:'
```

## Task 6 closed: systems from the Knowledge Base - 2026-10-31 (not committed)

Continues the plan `docs/superpowers/plans/2026-10-30-sbe-offers-materials-systems-plan.md`
after the cycle stop. This finishes Task 6 only; Tasks 7-9 (plugin UI, bot,
release) are untouched.

**What the task was.** The split between the two entities was decided by tags in
`matcher.go` (`splitCardsByTags`, commit `2f2d3c8`), but nothing read that rule:
there was no route, the composition was a list of plain strings with no link to
the material catalogue, and a line pointing at a material that does not exist
disappeared into the same string list with no trace.

**Server.**

- `skills/systemcard.go`: `SystemCard` gains `Links []CompositionLink{Raw, SKU}`,
  filled by `linksFromComposition` in `ParseSystemCard`. `Composition []string`
  stays exactly as it was - it feeds the model prompt and the document, and
  changing its type would ripple into DOCX and the brief. `ExtractSku` is exported
  so the service can derive the code from a card read out of the old cache. The
  regex has no `\b` and accepts four digit numbers; a `РАБ-` code is found (test
  `TestCompositionLinksWorkSku`), which is the third catch of that gotcha.
- `systemcards.go`: `CompositionItem{Raw, SKU, MaterialID, Name, Unit, Ekn, Found}`
  and `resolveComposition(card, materials)`. Every line ends up in the result;
  `Found=false` lines are named in `Problems` with the reason: no code, unknown
  code, or "material removed from the catalogue" (an archived material is found
  in the map but reported separately - "not found" would be a lie). An empty
  composition is one problem, not silently fine.
  `systemCardFromParsed` is now the single place that builds a card from a parsed
  note - `SystemCardsFromKB` and `SystemsFromKB` both use it.
- `SystemsFromKB` reads `commercial_propose`, splits by tag, keeps systems only.
- Route `GET /api/offers/systems` (`viewer`), handler `handleListSystems` answers
  `{systems, kbEnabled, count, error?}`. It reads the **Knowledge Base live**, not
  the card cache: the composition has to resolve against the *current* material
  catalogue, while the cache serves names to the offer context. A KB failure
  answers 200 with an empty list and `error`, as the refresh route does.
- `sbe-offers-service.PromptText` now delegates to `skills.SystemCard.PromptText`.
  The two were byte-identical (`dryer` 0.88-1.00 duplicate), and a prompt format
  kept in two places drifts silently.

**Stand.**

- `scripts/kb-stub.mjs`: a third note with tags `commercial_propose` + `material`
  (so the tag split is checked against the same feed), and the classic-roof card
  now has `- 1 Мембрана ПВХ Эксперт (MAT-МЕМБ-ПВХ-0001)` in its composition.
- `scripts/offers_local_e2e.mjs` section 17: tag split, composition resolved
  against the material created in section 16 (name, id, `found`), a code-less
  line staying visible and named in `problems`, 401 without a token.

**Checks.** `gofmt`/`go vet` clean; `go test ./...` green with `main` 40.7%,
`model` 71.6%, `skills` 84.2%. Coverage of the new code (`go tool cover -func`):
`resolveComposition` 90.6, `SystemsFromKB` 84.2, `systemCardFromParsed` 100,
`splitCardsByTags`/`noteHasTag` 100. `check_offers_openapi.mjs`: 56 routes, 44
paths, schema matches. Local stand E2E **171/171**, run twice. 15 tests added in
`skills/systemcard_links_test.go` and `systems_split_test.go`.

**Not done, by the plan's own boundary.** Plugin UI (Task 7), bot (Task 8),
version/docs/0.3.0 (Task 9). Committed and pushed later the same day as `6b1fc45`
(the entry was written before the commit, so the hash is named here rather than
in the commit it describes - the 2026-10-30 lesson).

**Stand lesson (cost an hour).** The local service could not be rebuilt from its
Dockerfile (same Docker Hub reachability as 2026-10-30), so the binary was built
in a `golang:1.24` image. A CGO-enabled Go binary copied over the running
container's `/mailers-offers` fails with `exec /mailers-offers: no such file or
directory` - the kernel needs `ld-linux-x86-64.so.2` and musl has only
`ld-musl-x86_64.so.1`, so the "file is not found" message is about the
*interpreter*, not the file. Build with `CGO_ENABLED=0` for the stand; the
container image itself is fine because it builds the binary on its own base.
Also: `docker cp` into a container whose binary is running replaces the file but
leaves that container's binary unmappable until it is restarted - restart after
cp, then check `docker ps` says `Up`, not `Exited (255)`.

## Task 7: the plugin caught up with the two entities - 2026-10-31

The plugin still had one section called "Каталог" with every card carrying a
price, while the server had already split materials (with SKU and ЕКН) from
systems (KB cards, no price). This closes that gap; the bot (Task 8) and the
release with 0.3.0 (Task 9) are still open.

**Navigation.** "Каталог" is gone, replaced by two sections. `catalog-editor.ts`
became `material-editor.ts` and `catalog-editor.ts` was deleted. The legacy
`/catalog` routes and the `offers_catalog` table stay on the server untouched:
old offers still point at those rows, and removing the routes would break them
for no gain.

**Materials (`ui/material-editor.ts`).**

- List: name with the SKU underneath in monospace, unit, price range, ЕКН count.
  No price column - a material has no single price, and a column would silently
  become "the" price the way `offers_catalog.base_price` once did.
- Search covers name, SKU and ЕКН numbers together: managers look a material up
  by the number printed on the spec sheet, not by the catalogue name.
- "Показать архив" is a checkbox, off by default; archived rows are dimmed.
- Card: name, kind (material/work), unit, description. SKU is shown and never
  editable - it is an identity, assigned once by the server.
- New material: two extra fields, "Тип" and "Серия", which only feed the SKU
  generator (`MAT-{TYPE}-{SERIES}-{NNNN}`). They are hidden when editing,
  because a rename never changes the code.
- ЕКН table with number, variant, source (excel/manual) and the last seen date;
  add and delete inside the same modal.
- `priceRangeText` prints "цена не загружалась" when both ends are `null`. Zero
  is a legitimate price and a different thing.

**Systems (`ui/systems-editor.ts`).**

- One card per system: title, code, and either "состав распознан" or "есть
  ошибки в составе". Problems are listed as text - a line without a code, a code
  that is not in the catalogue, a deleted material.
- The composition table shows each line's material, unit and ЕКН, so a manager
  can see what the system is made of without opening the note.
- "Открыть заметку" reuses `openNote` from the context editor (exported, not
  copied): the KB path is logical and the file lives elsewhere in the vault; the
  resolution rule lives in `utils/note-path.ts` and must not have a second copy.
- `formatShortDate` moved to `utils/date.ts` for the same reason - it was about to
  become a third copy.

**Sheet upload (`ui/sheet-upload.ts`).**

- Only for a saved offer: the sheet writes positions into a specific offer and
  moves its `updated_at`.
- File input, then "what I understood": counts per matching method, the problems
  the server found, and a table with a checkbox per row.
- **Rows matched by the model are NOT checked by default.** That is the whole
  point of the two-step flow: a guess needs a human click. Exact ЕКН and exact
  name hits are checked.
- Quantity defaults to 1 only when the column is absent (`hasQty`), never
  silently for a file that says 0.
- The file is read before `input.value` is cleared - the live `FileList` gotcha
  from sbe-mailer 2026-09-18.
- `.xlsx` only; `.xls` gets a message asking to resave.

**"Добавить систему" in the offer editor.**

- This needed a server route: `POST /api/offers/offers/{id}/system` (editor).
  The price of a position is known only on the server (the range seen in the
  manager's sheets), so client-side assembly would have to invent numbers.
- The composition is **appended** to the existing items, not a replacement: a
  system is a set the manager wanted to add.
- Price = the *lower* bound of the range. No range means price 0 and the material
  is named in `missingPrice`, which the UI shows in the notice - the manager has
  to load a sheet for it.
- Lines that resolve to no material are returned in `problems` and never reach
  the offer silently.
- The system is looked up in the live KB feed (the same call the "Системы"
  section makes), not in the card cache, so what the manager saw on screen is
  what gets added.
- `FindSystemCard` became a thin wrapper over the new `FindSystem` - one rule for
  comparing codes instead of two.

**Defects found by the stand, not by unit tests.**

1. An empty offer cannot be created (the server refuses offers without
   positions), so the E2E had to start from a one-line "Заглушка" offer. That is
   correct server behaviour, the test was wrong.
2. The E2E sheet named ЕКН `068863`, which section 16 deletes at the end of its
   run. The preview showed `byEkn: 0` and the row fell through to the model. The
   test now uses what the catalogue actually holds at that point (`068864`,
   "Мембрана ПВХ Эксперт 2") - a fixture that drifts from real state is a bug in
   the fixture.
3. `handleAddSystem` came out at CC 13 with zero coverage. The loop was moved to
   `appendComposition` (now 100% covered) and `handleAddSystem` dropped to 8.

**Checks.** `gofmt`/`go vet` clean. `go test ./...` green: `main` 40.6%,
`model` 71.6%, `skills` 84.2%; new code - `appendComposition` 100,
`itemForMaterial` 100, `FindSystem` 100, `handleAddSystem` 0 (HTTP, covered by
the stand like every other route in this plugin). `tsc --noEmit` EXIT=0,
`npm run build` OK. `check_offers_openapi.mjs`: 57 routes, 45 paths, schema
matches. Local stand E2E **189/189** (section 18: sheet preview writes nothing,
apply takes the price from the sheet, system appends rather than replaces,
unknown system 404, no token 401), run twice.

**Not done.** Marked as Task 7 complete in the plan. Bot (Task 8), version 0.3.0
with the release notes, deploy and self-hosting (Task 9) remain; nothing was
deployed or self-hosted for this change.