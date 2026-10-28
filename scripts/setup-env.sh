#!/usr/bin/env bash
# Creates or refills .env from .env.example.
#
#   scripts/setup-env.sh          ask for each secret (Enter keeps what is there)
#   scripts/setup-env.sh --yes    ask nothing: keep current values, generate what is missing
#
# An existing .env is first copied to .env.bak.<timestamp>. Its values become the defaults, and
# keys the template does not know are carried over at the end, so nothing is lost. A variable
# set in the environment wins over both (TELEGRAM_BOT_TOKEN=… scripts/setup-env.sh --yes).
# POSTGRES_PASSWORD is generated once and then kept: the database only takes it when created.
set -euo pipefail

if ((BASH_VERSINFO[0] < 4)); then
  echo "needs bash 4 or newer" >&2
  exit 1
fi

cd "$(dirname "$0")/.."
template=.env.example
target=.env
yes=0
[[ "${1:-}" == "--yes" || "${1:-}" == "-y" ]] && yes=1
[[ -t 0 ]] || yes=1 # no terminal to ask on

[[ -f $template ]] || { echo "$template is missing" >&2; exit 1; }

# keys asked for, in this order; secrets are read without echo
ask=(PUBLIC_URL TELEGRAM_BOT_TOKEN ADMIN_TELEGRAM_IDS OPENROUTER_API_KEY TONAPI_KEY TON_WALLET PROXIES WEB_PORT)
declare -A secret=([TELEGRAM_BOT_TOKEN]=1 [OPENROUTER_API_KEY]=1 [TONAPI_KEY]=1 [PROXIES]=1)
declare -A help=(
  [PUBLIC_URL]="public https address of the app (the reverse proxy's domain)"
  [TELEGRAM_BOT_TOKEN]="bot token from @BotFather"
  [ADMIN_TELEGRAM_IDS]="your Telegram id(s), comma-separated: the admins (set it, or the first sign-in is admin)"
  [OPENROUTER_API_KEY]="OpenRouter key for the AI features"
  [TONAPI_KEY]="tonapi.io key (optional, faster chain crawl)"
  [TON_WALLET]="wallet for TON top-ups (empty = TON payments off)"
  [PROXIES]="outbound proxies, comma-separated (optional)"
  [WEB_PORT]="port on 127.0.0.1 the reverse proxy passes to"
)

unquote() {
  local v=$1
  if [[ ${#v} -ge 2 && ( ( $v == \"*\" && $v == *\" ) || ( $v == \'*\' && $v == *\' ) ) ]]; then
    v=${v:1:${#v}-2}
  fi
  printf '%s' "$v"
}

# Quote values the .env readers would otherwise mangle: compose expands $, and # starts a comment.
quote() {
  local v=$1
  if [[ $v =~ [[:space:]\#\$\"\\\`] ]]; then
    printf "'%s'" "$v"
  else
    printf '%s' "$v"
  fi
}

declare -A old=()
declare -a old_keys=()
if [[ -f $target ]]; then
  while IFS= read -r line || [[ -n $line ]]; do
    line=${line%$'\r'}
    [[ $line =~ ^[[:space:]]*([A-Za-z_][A-Za-z0-9_]*)[[:space:]]*=(.*)$ ]] || continue
    k=${BASH_REMATCH[1]}
    v=${BASH_REMATCH[2]}
    v="${v#"${v%%[![:space:]]*}"}"
    v="${v%"${v##*[![:space:]]}"}"
    [[ -v old[$k] ]] || old_keys+=("$k")
    old[$k]=$(unquote "$v")
  done <"$target"
fi

# defaults: template, then the old .env, then the environment
declare -A val=()
declare -a keys=()
while IFS= read -r line || [[ -n $line ]]; do
  line=${line%$'\r'}
  [[ $line =~ ^([A-Za-z_][A-Za-z0-9_]*)=(.*)$ ]] || continue
  k=${BASH_REMATCH[1]}
  keys+=("$k")
  val[$k]=$(unquote "${BASH_REMATCH[2]}")
  [[ -v old[$k] ]] && val[$k]=${old[$k]}
  [[ -n ${!k-} ]] && val[$k]=${!k}
done <"$template"

mask() {
  local v=$1
  if [[ -z $v ]]; then echo "empty"; elif ((${#v} <= 8)); then echo "set"; else echo "${v:0:4}…${v: -4}"; fi
}

if ((yes == 0)); then
  echo
  echo "Enter keeps the value in [brackets]; a single - clears it."
  for k in "${ask[@]}"; do
    cur=${val[$k]-}
    if [[ -n ${secret[$k]-} ]]; then
      read -r -s -p "$k — ${help[$k]} [$(mask "$cur")]: " in
      echo
    else
      read -r -p "$k — ${help[$k]} [$cur]: " in
    fi
    if [[ $in == "-" ]]; then
      val[$k]=""
    elif [[ -n $in ]]; then
      val[$k]=$in
    fi
  done
fi

# --- checks and generated values ---
if [[ -n ${val[PUBLIC_URL]-} && ! ${val[PUBLIC_URL]} =~ ^https?:// ]]; then
  echo "PUBLIC_URL must start with https:// (or http:// for local use)" >&2
  exit 1
fi
[[ ${val[PUBLIC_URL]-} == https://* ]] || echo "note: PUBLIC_URL is not https; the Mini App and the Login Widget need https"
if [[ -n ${val[TELEGRAM_BOT_TOKEN]-} && ! ${val[TELEGRAM_BOT_TOKEN]} =~ ^[0-9]+:[A-Za-z0-9_-]{30,}$ ]]; then
  echo "TELEGRAM_BOT_TOKEN does not look like a bot token (123456:ABC…)" >&2
  exit 1
fi
if [[ -n ${val[WEB_PORT]-} && ! ${val[WEB_PORT]} =~ ^[0-9]+$ ]]; then
  echo "WEB_PORT must be a number" >&2
  exit 1
fi

gen() {
  if command -v openssl >/dev/null; then
    openssl rand -hex 24
  else
    LC_ALL=C tr -dc 'a-f0-9' </dev/urandom | head -c 48
  fi
}
if [[ -z ${val[POSTGRES_PASSWORD]-} ]]; then
  val[POSTGRES_PASSWORD]=$(gen)
  echo "Generated POSTGRES_PASSWORD"
elif [[ -n ${old[POSTGRES_PASSWORD]-} ]]; then
  echo "Kept POSTGRES_PASSWORD (the database was created with it)"
fi
# for `go run` against docker-compose.dev.yml; always rebuilt from the values above
val[DATABASE_URL]="postgres://${val[POSTGRES_USER]}:${val[POSTGRES_PASSWORD]}@127.0.0.1:${val[DB_PORT]:-5432}/${val[POSTGRES_DB]}?sslmode=disable"

# --- write: the template's layout and comments, with the values filled in ---
tmp=$(mktemp "$target.XXXXXX")
trap 'rm -f "$tmp"' EXIT
chmod 600 "$tmp"
{
  while IFS= read -r line || [[ -n $line ]]; do
    line=${line%$'\r'}
    if [[ $line =~ ^([A-Za-z_][A-Za-z0-9_]*)= ]]; then
      k=${BASH_REMATCH[1]}
      printf '%s=%s\n' "$k" "$(quote "${val[$k]-}")"
    else
      printf '%s\n' "$line"
    fi
  done <"$template"
  extra=()
  declare -A known=()
  for k in "${keys[@]}"; do known[$k]=1; done
  for k in "${old_keys[@]}"; do [[ -n ${known[$k]-} ]] || extra+=("$k"); done
  if ((${#extra[@]})); then
    printf '\n# --- kept from the previous .env (not in %s) ---\n' "$template"
    for k in "${extra[@]}"; do printf '%s=%s\n' "$k" "$(quote "${old[$k]}")"; done
  fi
} >"$tmp"
if [[ -f $target ]]; then
  backup="$target.bak.$(date +%Y%m%d-%H%M%S)"
  cp -p "$target" "$backup"
  chmod 600 "$backup"
  echo "Backed up the previous $target to $backup"
fi
mv "$tmp" "$target"
trap - EXIT
echo "Wrote $target (mode 600)"

# the bot token is checked with Telegram itself, the only place it is ever sent
if [[ -n ${val[TELEGRAM_BOT_TOKEN]-} ]] && command -v curl >/dev/null; then
  if me=$(curl -fsS --max-time 10 "https://api.telegram.org/bot${val[TELEGRAM_BOT_TOKEN]}/getMe" 2>/dev/null); then
    bot=$(printf '%s' "$me" | sed -n 's/.*"username":"\([^"]*\)".*/\1/p')
    echo "Telegram bot: @${bot:-?}"
  else
    echo "warning: Telegram did not accept TELEGRAM_BOT_TOKEN (or could not be reached)" >&2
  fi
fi

echo
echo "Next: docker compose up -d --build"
