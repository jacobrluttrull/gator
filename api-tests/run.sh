#!/usr/bin/env bash
#
# Runs the whole gator API flow end to end and prints a pass/fail table.
# Same requests, same order as gator.http — this is the version that works
# anywhere, including Zed's terminal, with nothing installed but curl.
#
#   1. Start the server:  gator serve -port 8080 -interval 1m
#   2. Edit NAME and PASSWORD below (that's it).
#   3. ./api-tests/run.sh
#
# Any setting can also be overridden per-run without editing the file:
#   NAME=bob PASSWORD=s3cret ./api-tests/run.sh
#   BASE=http://localhost:8099 ./api-tests/run.sh
#
# Exits 0 if everything passed, 1 if anything failed, 2 if the server is down.

set -uo pipefail

# ─────────── EDIT THESE TWO ───────────

NAME=${NAME:-alice}
PASSWORD=${PASSWORD:-hunter2}

# ─────────── the rest rarely changes ───────────

BASE=${BASE:-http://localhost:8080}
FEED_NAME=${FEED_NAME:-Hacker News}
FEED_URL=${FEED_URL:-https://news.ycombinator.com/rss}
SEARCH=${SEARCH:-go}

# ─────────── plumbing ───────────

if [ -t 1 ]; then
    GREEN=$'\033[32m'; RED=$'\033[31m'; DIM=$'\033[2m'; BOLD=$'\033[1m'; OFF=$'\033[0m'
else
    GREEN=''; RED=''; DIM=''; BOLD=''; OFF=''
fi

PASSED=0
FAILED=0
BODY=''
STATUS=''

# req <label> <expected-codes> <curl args...>
# Runs a request, prints one table row, leaves the response in $BODY.
# <expected-codes> is a space-separated list, e.g. "201 409" when either is fine.
req() {
    local label=$1 expected=$2; shift 2
    local tmp; tmp=$(mktemp)
    STATUS=$(curl -sS -o "$tmp" -w '%{http_code}' "$@" 2>/dev/null)
    BODY=$(cat "$tmp"); rm -f "$tmp"

    local ok=no code
    for code in $expected; do
        [ "$STATUS" = "$code" ] && ok=yes
    done

    if [ "$ok" = yes ]; then
        PASSED=$((PASSED + 1))
        printf '%-22s %s%3s ✓%s  %s%s%s\n' "$label" "$GREEN" "$STATUS" "$OFF" "$DIM" "${NOTE:-}" "$OFF"
    else
        FAILED=$((FAILED + 1))
        printf '%-22s %s%3s ✗%s  %sexpected %s%s\n' "$label" "$RED" "$STATUS" "$OFF" "$RED" "$expected" "$OFF"
        printf '%24s%s%s%s\n' '' "$DIM" "$(printf '%s' "$BODY" | head -c 160)" "$OFF"
    fi
    NOTE=''
}

# Pull one string field out of $BODY. The API's JSON is flat and machine-
# generated, so grep is enough — no jq or python needed.
field() { printf '%s' "$BODY" | grep -o "\"$1\":\"[^\"]*\"" | head -1 | cut -d'"' -f4; }
count() { printf '%s' "$BODY" | grep -o '"id":"' | wc -l | tr -d ' '; }

json() { echo "Content-Type: application/json"; }

# JSON-escape a string for safe embedding inside a "..." body literal — quote
# and backslash are the two bytes that break the surrounding JSON, plus control
# characters (newline, carriage return, tab) that must be encoded.
json_escape() {
    local s=$1
    s=${s//\\/\\\\}
    s=${s//\"/\\\"}
    s=${s//$'\n'/\\n}
    s=${s//$'\r'/\\r}
    s=${s//$'\t'/\\t}
    printf '%s' "$s"
}

NAME_JSON=$(json_escape "$NAME")
PASSWORD_JSON=$(json_escape "$PASSWORD")
FEED_NAME_JSON=$(json_escape "$FEED_NAME")
FEED_URL_JSON=$(json_escape "$FEED_URL")

# ─────────── preflight ───────────

printf '%sgator API flow%s  %s%s as %s%s\n\n' "$BOLD" "$OFF" "$DIM" "$BASE" "$NAME" "$OFF"

if ! curl -sS -o /dev/null --max-time 5 "$BASE/v1/feeds" 2>/dev/null; then
    printf '%sCannot reach %s%s\n' "$RED" "$BASE" "$OFF"
    printf 'Start the server first:  gator serve -port %s -interval 1m\n' "${BASE##*:}"
    exit 2
fi

# ─────────── the flow ───────────

# 1. Create a user WITH a password, so they can log in over HTTP. (The CLI's
#    `gator register` does NOT set one — that makes a CLI-only user.)
#    409 just means you have run this before.
req "1. register" "201 409" "$BASE/v1/register" -H "$(json)" \
    -d "{\"name\":\"$NAME_JSON\",\"password\":\"$PASSWORD_JSON\"}"
[ "$STATUS" = 409 ] && printf '%24s%suser already exists — fine, continuing%s\n' '' "$DIM" "$OFF"

# 2. Verify the password and get an API key. This is the only time the
#    plaintext key is ever shown; the DB stores only its SHA-256 hash.
req "2. login" "200" "$BASE/v1/login" -H "$(json)" \
    -d "{\"name\":\"$NAME_JSON\",\"password\":\"$PASSWORD_JSON\"}"
KEY=$(field api_key)
if [ -z "$KEY" ]; then
    printf '\n%sNo API key returned — everything below needs it, stopping.%s\n' "$RED" "$OFF"
    printf '%sA 401 here means wrong password, unknown user, or a CLI-only user with\n' "$DIM"
    printf 'no password set. Fix with:  gator login %s && gator setpassword %s%s\n' "$NAME" "$PASSWORD" "$OFF"
    exit 1
fi
printf '%24s%skey %s… captured, reused below%s\n' '' "$DIM" "${KEY:0:12}" "$OFF"
AUTH="Authorization: ApiKey $KEY"

# 3. Add the feed to the shared global pool AND follow it, in one step.
#    Feeds exist once for everybody, fetched once no matter how many follow.
req "3. add feed" "201 409" "$BASE/v1/feeds" -H "$AUTH" -H "$(json)" \
    -d "{\"name\":\"$FEED_NAME_JSON\",\"url\":\"$FEED_URL_JSON\"}"
[ "$STATUS" = 409 ] && printf '%24s%sfeed URL already in the pool — fine, step 6 follows it%s\n' '' "$DIM" "$OFF"

# 4. Every feed anyone has added, with who added it — not just yours.
req "4. list feeds" "200" "$BASE/v1/feeds" -H "$AUTH"

# 5. Unfollow by the feed's URL. Idempotent, so this is safe even on a first
#    run. It goes before the follow purely so the script reruns cleanly.
req "5. unfollow" "204" -X DELETE "$BASE/v1/follows" -H "$AUTH" -H "$(json)" \
    -d "{\"url\":\"$FEED_URL_JSON\"}"

# 6. Follow a feed that already exists in the pool. This is what you use for
#    somebody else's feed — step 3 only works for a brand new URL.
req "6. follow" "201" "$BASE/v1/follows" -H "$AUTH" -H "$(json)" \
    -d "{\"url\":\"$FEED_URL_JSON\"}"

# 7. Just the feeds you personally follow.
req "7. list follows" "200" "$BASE/v1/follows" -H "$AUTH"

# 8. Posts from feeds you follow, newest first. Empty until the aggregator has
#    fetched — it does ONE feed per interval, so this can take a few minutes.
req "8. browse posts" "200" "$BASE/v1/posts?limit=10" -H "$AUTH"
POSTS=$(count)
POST_URL=$(field url)
printf '%24s%s%s post(s)%s\n' '' "$DIM" "$POSTS" "$OFF"

if [ -z "$POST_URL" ]; then
    printf '\n%sNo posts yet, so steps 9–11 (bookmarks) are skipped.%s\n' "$DIM" "$OFF"
    printf '%sThe aggregator fetches one feed per interval — give it a minute and\n' "$DIM"
    printf 'rerun, or check the gator serve log for scrape errors.%s\n\n' "$OFF"
else
    # 9. Bookmark by the POST's url (not the feed's). Bookmarks are private.
    req "9. bookmark" "201 409" "$BASE/v1/bookmarks" -H "$AUTH" -H "$(json)" \
        -d "{\"url\":\"$POST_URL\"}"

    # 10. Returns the posts themselves, newest bookmark first — not join rows.
    req "10. list bookmarks" "200" "$BASE/v1/bookmarks" -H "$AUTH"

    # 11. Remove it again.
    req "11. unbookmark" "204" -X DELETE "$BASE/v1/bookmarks" -H "$AUTH" -H "$(json)" \
        -d "{\"url\":\"$POST_URL\"}"
fi

# 12. Fuzzy trigram match on post titles, across feeds you follow only. Each
#     hit carries a "sim" score. A blank q would be a 400, not an empty list.
#
#     0 hits is NOT a failure. similarity() compares the whole title to the
#     whole term against a 0.3 threshold, so one short word out of a long title
#     scores too low to match even when it is literally in the title. Use more
#     words.
req "12. search" "200" --get "$BASE/v1/search" --data-urlencode "q=$SEARCH" -d "limit=10" -H "$AUTH"
printf '%24s%s%s hit(s) for %s%s\n' '' "$DIM" "$(count)" "$SEARCH" "$OFF"

# 13. Kills EVERY key you hold, including the one making this request. Keys
#     never expire, so revoking is the only way one ever stops working.
req "13. revoke keys" "204" -X DELETE "$BASE/v1/keys" -H "$AUTH"

# 14. Same request as step 8, same key — must now be rejected.
req "14. prove revoked" "401" "$BASE/v1/posts" -H "$AUTH"

# ─────────── error-shape checks ───────────
#
# These need a live key again, since step 13 just killed it. Auth is checked
# before anything else, so without this re-login they would all return 401
# instead of the 404/405/400 they are meant to demonstrate.

req "    (re-login)" "200" "$BASE/v1/login" -H "$(json)" \
    -d "{\"name\":\"$NAME_JSON\",\"password\":\"$PASSWORD_JSON\"}"
AUTH="Authorization: ApiKey $(field api_key)"

printf '\n%serror shapes%s\n' "$BOLD" "$OFF"

# No header at all.
req "no auth header" "401" "$BASE/v1/posts"

# Wrong scheme. It must be exactly "ApiKey <key>" — not Bearer, not lowercase.
# This is the single most common cause of a mystery 401.
req "Bearer not ApiKey" "401" "$BASE/v1/posts" -H "${AUTH/ApiKey/Bearer}"

# Unknown path → JSON 404, not the stdlib's plain-text page.
req "unknown path" "404" "$BASE/v1/nope" -H "$AUTH"

# Real path, wrong verb → JSON 405 with an accurate Allow header.
req "wrong verb (PUT)" "405" -X PUT "$BASE/v1/posts" -H "$AUTH"
ALLOW=$(curl -sSI -X PUT "$BASE/v1/posts" -H "$AUTH" 2>/dev/null | grep -i '^allow:' | tr -d '\r')
printf '%24s%s%s%s\n' '' "$DIM" "${ALLOW:-no Allow header!}" "$OFF"

# Zero, negative, or past int32 → 400, never a wrapped negative LIMIT.
req "limit=0" "400" "$BASE/v1/posts?limit=0" -H "$AUTH"

# ─────────── result ───────────

printf '\n'
if [ "$FAILED" -eq 0 ]; then
    printf '%sall %d passed%s\n' "$GREEN" "$PASSED" "$OFF"
    exit 0
fi
printf '%s%d passed, %d failed%s\n' "$RED" "$PASSED" "$FAILED" "$OFF"
exit 1
