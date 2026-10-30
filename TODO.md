# To do

## Done (2026-10-30, not yet checked in the app)

- **Duplicate "new names" alerts.** Each account remembers which names it was told about
  (`alerts:told:<name>`). A name comes back only after a ≥10% drop or a week. A GetGems listing,
  a higher price or a sale is no longer news.
- **A name in both "new" and "price drops" in one run.** It goes in "drops" only.
- **Lost alerts on a failed send.** `last_run` and drop prices are saved only after the messages go
  out. A run never looks back more than a day.
- **Ideas repeating the same names.** Names from the last ~5 sets wait while there are others, and
  the list the model sees is shuffled.
- **Where ideas take names from.** A "Names from" picker (taste / favorites / snipes / a tracker)
  shows how many names the source holds and how many the AI sees per set.

## Now

- Check in the app with a demo account: the source picker on Ideas, ten presses of "New set",
  then re-shoot `ideas-studio.png` (`docs/guides/shoot.mjs`).
- **Tracker "new" badge** (`trackers.go`) still counts a GetGems listing or a reprice as new. Same
  cause as the alerts, lower stakes: it's a counter, not a ping.

## Next
- Show every venue on an alert card when a name is listed in several places ("also on GetGems").
- Mute a name from the alert itself (a "🔕" button next to each name).
- Digest mode: one message every N hours instead of every 5 minutes, plus quiet hours.
- Choose which venues to be alerted about (Fragment only, or Fragment + marketplaces).
- Ideas: pick several sources at once (e.g. favorites + one tracker).

## Later
- History of sent alerts in the app, so "why did I get this?" has an answer.
