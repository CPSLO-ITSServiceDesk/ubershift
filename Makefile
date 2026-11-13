dev:
	air
# run the software
run:
	go run ./cmd/bot

# Post one Monday-Friday schedule, then exit. Example: make post-schedule DATE=2026-10-27
post-schedule:
	@test -n "$(DATE)" || (echo "Usage: make post-schedule DATE=YYYY-MM-DD" && exit 1)
	UBERSHIFT_POST_ONCE=true UBERSHIFT_SCHEDULE_DATE="$(DATE)" go run ./cmd/bot

# reformats Go source code using gofmt
format:
	gofmt -w .

# runs unit tests
test:
	go test ./...

# is the code safe to push
check:
	gofmt -w .
	go vet ./...
	go test ./...
	go build ./...
