#!/bin/sh
set -u

api_url=${API_URL:-http://api:8080}
account=small_account
mebibyte=1048576
run_id=$(date +%s)-$$
work_directory=$(mktemp -d)
pending_reservations="$work_directory/pending"
: > "$pending_reservations"
step_number=0

release_pending_reservations() {
	exit_status=$?
	if [ -s "$pending_reservations" ]; then
		printf '\nReleasing the reservations this run created, so the demo can run again.\n'
		while read -r reservation_id; do
			curl -s -o /dev/null -X POST "$api_url/accounts/$account/reservations/$reservation_id/release"
		done < "$pending_reservations"
	fi
	rm -rf "$work_directory"
	exit "$exit_status"
}
trap release_pending_reservations EXIT

send() {
	method=$1
	path=$2
	idempotency_key=${3:-}
	body=${4:-}
	set -- -s -o "$work_directory/body" -D "$work_directory/headers" -w '%{http_code}' -X "$method"
	if [ -n "$idempotency_key" ]; then
		set -- "$@" -H "Idempotency-Key: $idempotency_key"
	fi
	if [ -n "$body" ]; then
		set -- "$@" -H 'Content-Type: application/json' -d "$body"
	fi
	curl "$@" "$api_url$path"
}

reserve_body() {
	printf '{"bytes": %s}' "$(($1 * mebibyte))"
}

json_number() {
	sed -n "s/.*\"$1\":\([0-9][0-9]*\).*/\1/p" "$work_directory/body"
}

json_text() {
	sed -n "s/.*\"$1\":\"\([^\"]*\)\".*/\1/p" "$work_directory/body"
}

response_header() {
	tr -d '\r' < "$work_directory/headers" | sed -n "s/^$1: //p"
}

step() {
	step_number=$((step_number + 1))
	printf '\n%s. %s\n' "$step_number" "$1"
}

expect() {
	printf '   %s: expected %s, got %s\n' "$1" "$2" "$3"
	if [ "$2" != "$3" ]; then
		printf '\nFAILED at step %s: %s\n' "$step_number" "$1"
		if [ -n "${4:-}" ]; then
			printf '%s\n' "$4"
		fi
		exit 1
	fi
}

expect_usage() {
	expect "usage status" 200 "$(send GET "/accounts/$account/usage")"
	expect "reserved bytes" "$1" "$(json_number reserved_bytes)" "${3:-}"
	expect "active reservations" "$2" "$(json_number active_reservations)" "${3:-}"
}

forget_reservation() {
	grep -v -x "$1" "$pending_reservations" > "$work_directory/pending.next"
	mv "$work_directory/pending.next" "$pending_reservations"
}

printf 'Demo against %s, account %s, run %s\n' "$api_url" "$account" "$run_id"

step "Usage of $account before the demo"
expect_usage 0 0 "The demo needs an account with nothing reserved: reset the data with docker compose down -v."
expect "limit bytes" "$((100 * mebibyte))" "$(json_number limit_bytes)" "The demo expects small_account at its default limit of 100 MiB."
max_active_reservations=$(json_number max_active_reservations)
if [ -z "$max_active_reservations" ]; then
	printf '\nFAILED at step %s: MAX_PARALLEL_CHUNKS is not known yet; check that the service can reach ENV_CONFIG_URL.\n' "$step_number"
	exit 1
fi
printf '   at most %s active reservations (MAX_PARALLEL_CHUNKS)\n' "$max_active_reservations"

first_key="demo-$run_id-first"
step "Reserve 30 MiB with Idempotency-Key $first_key"
expect "status" 201 "$(send POST "/accounts/$account/reservations" "$first_key" "$(reserve_body 30)")"
first_reservation_id=$(json_text id)
echo "$first_reservation_id" >> "$pending_reservations"
expect "reservation status" active "$(json_text status)"
printf '   reservation %s, Location %s\n' "$first_reservation_id" "$(response_header Location)"

step "Send the same request again with the same key, as a client does after a lost response"
expect "status" 200 "$(send POST "/accounts/$account/reservations" "$first_key" "$(reserve_body 30)")"
expect "Idempotent-Replayed" true "$(response_header Idempotent-Replayed)"
expect "reservation id" "$first_reservation_id" "$(json_text id)"
expect_usage "$((30 * mebibyte))" 1

step "Reserve 80 MiB while only 70 MiB are free"
expect "status" 409 "$(send POST "/accounts/$account/reservations" "demo-$run_id-too-large" "$(reserve_body 80)")"
expect "code" insufficient_storage "$(json_text code)"
printf '   detail: %s\n' "$(json_text detail)"

expected_created=$((max_active_reservations - 1))
if [ "$expected_created" -gt 3 ]; then
	expected_created=3
fi
step "Send 20 reservations of 20 MiB at the same time, with distinct keys"
for request_number in $(seq 1 20); do
	curl -s -o "$work_directory/concurrent-body-$request_number" -w '%{http_code}' -X POST \
		-H "Idempotency-Key: demo-$run_id-concurrent-$request_number" -H 'Content-Type: application/json' \
		-d "$(reserve_body 20)" "$api_url/accounts/$account/reservations" \
		> "$work_directory/concurrent-status-$request_number" &
done
wait
created=0
refused=0
for request_number in $(seq 1 20); do
	case $(cat "$work_directory/concurrent-status-$request_number") in
	201)
		created=$((created + 1))
		sed -n 's/.*"id":"\([^"]*\)".*/\1/p' "$work_directory/concurrent-body-$request_number" >> "$pending_reservations"
		;;
	409)
		refused=$((refused + 1))
		;;
	esac
done
expect "201 Created" "$expected_created" "$created"
expect "409 Conflict" "$((20 - expected_created))" "$refused"
expect_usage "$(((30 + 20 * expected_created) * mebibyte))" "$((1 + expected_created))"

step "Reuse the first key with 31 MiB instead of 30"
expect "status" 422 "$(send POST "/accounts/$account/reservations" "$first_key" "$(reserve_body 31)")"
expect "code" idempotency_key_reused "$(json_text code)"
printf '   detail: %s\n' "$(json_text detail)"

step "Release the first reservation twice"
release_path="/accounts/$account/reservations/$first_reservation_id/release"
expect "first release status" 200 "$(send POST "$release_path")"
expect "reservation status" released "$(json_text status)"
released_at=$(json_text released_at)
forget_reservation "$first_reservation_id"
expect "second release status" 200 "$(send POST "$release_path")"
expect "released_at" "$released_at" "$(json_text released_at)"
expect_usage "$((20 * expected_created * mebibyte))" "$expected_created"

step "Release the reservations made in step 5"
for reservation_id in $(cat "$pending_reservations"); do
	expect "release $reservation_id" 200 "$(send POST "/accounts/$account/reservations/$reservation_id/release")"
	forget_reservation "$reservation_id"
done

step "Usage of $account after the demo"
expect_usage 0 0

printf '\nDemo passed: every step got the expected answer.\n'
