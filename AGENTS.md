# Notes for agents

## User guides (`docs/guides/`)

Russian how-to guides with screenshots, published by hand to Telegraph. Keep them true to the app.

When a change alters something a guide describes, update the guide in the same change:

| Code | Guide |
|---|---|
| `frontend/components/Snipes.tsx`, `SettingsDialog.tsx`, `FilterBar.tsx`, `AIFilter.tsx`, their `lib/i18n/ru/*` | `snipes.md` |
| `frontend/components/Ideas.tsx`, `lib/i18n/ru/ideas.ts` | `ideas.md` |
| `frontend/components/AskAI.tsx`, `ConceptChat.tsx`, `AIFilter.tsx`, credit prices in `backend/internal/app/credits.go` | `ai-chat.md` |

What counts: a renamed button or section, a new or removed setting or action, a changed price in credits, a changed flow. Pure refactors and styling tweaks don't.

How:
1. Edit the text; keep to Telegraph-safe markdown (see `docs/guides/README.md`).
2. If the screen changed visibly, re-shoot the affected scenes with `docs/guides/shoot.mjs` (usage in `docs/guides/README.md`; needs the app running and a demo account's session). If a scene breaks because the UI moved, fix the scene. Look at every new PNG before keeping it.
3. Set today's date in the "Обновлён" column of `docs/guides/README.md`.
4. End your reply to the user with a line like: **📌 Гайд `snipes.md` обновлён — перезалейте его в Telegraph.** The user publishes; never post to Telegraph yourself.
