# run the software
run:
	go run .

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