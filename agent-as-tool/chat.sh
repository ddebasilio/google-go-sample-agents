#!/usr/bin/env bash
# Interactive REPL for the deployed ADK agent on Cloud Run.
# Usage: ./chat.sh
#   /new   start a fresh session
#   /exit  quit (Ctrl-D also works)

set -u

URL="${ADK_URL:-https://adk-default-service-name-841572325613.europe-west3.run.app}"
APP="${ADK_APP:-agent}"
USER_ID="${ADK_USER:-u1}"

command -v jq >/dev/null || { echo "jq is required"; exit 1; }
command -v gcloud >/dev/null || { echo "gcloud is required"; exit 1; }

refresh_token() { TOKEN=$(gcloud auth print-identity-token); }

spinner_start() {
  (
    SECONDS=0
    chars='|/-\'
    i=0
    while :; do
      printf '\r%s thinking… %ds ' "${chars:i++%4:1}" "$SECONDS" >&2
      sleep 0.2
    done
  ) &
  SPIN_PID=$!
  disown "$SPIN_PID" 2>/dev/null || true
}

spinner_stop() {
  [[ -n "${SPIN_PID:-}" ]] || return
  kill "$SPIN_PID" 2>/dev/null
  wait "$SPIN_PID" 2>/dev/null
  SPIN_PID=
  printf '\r\033[K' >&2
}

new_session() {
  SESSION="s$(date +%s)"
  curl -s -X POST -H "Authorization: Bearer $TOKEN" \
    "$URL/apps/$APP/users/$USER_ID/sessions/$SESSION" >/dev/null
  echo "[session: $SESSION]"
}

send() {
  local msg=$1
  local body
  body=$(jq -nc --arg a "$APP" --arg u "$USER_ID" --arg s "$SESSION" --arg m "$msg" \
    '{appName:$a,userId:$u,sessionId:$s,newMessage:{role:"user",parts:[{text:$m}]}}')

  local resp http
  spinner_start
  resp=$(curl -s -w "\n%{http_code}" -X POST \
    -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
    "$URL/run" -d "$body")
  spinner_stop
  http=$(printf '%s' "$resp" | tail -n1)
  resp=$(printf '%s' "$resp" | sed '$d')

  if [[ "$http" == "401" || "$http" == "403" ]]; then
    echo "[auth $http — refreshing token]"; refresh_token; send "$msg"; return
  fi
  if [[ "$http" != "200" ]]; then
    echo "[HTTP $http] $resp"; return
  fi

  printf '%s' "$resp" | jq -r '
    .[] |
    if .errorMessage then "[error] " + .errorMessage
    else
      (.author // "agent") as $a |
      (.content.parts[]? |
        if .text then .text
        elif .functionCall then "[\($a) → " + .functionCall.name + "()]"
        elif .functionResponse then "[\($a) ← " + .functionResponse.name + "]"
        else empty end)
    end' | sed '/^$/d'
}

refresh_token
new_session
echo "Chatting with $APP at $URL"
echo "Type /new to reset, /exit to quit."

while IFS= read -r -e -p "> " line; do
  case "$line" in
    "")       continue ;;
    /exit|/quit) break ;;
    /new)     new_session ;;
    *)        send "$line" ;;
  esac
done
echo
