BINARY_NAME=main
DIST_FOLDER=./dist

.PHONY: build

run-clean:
	rm data/state.json
	go run cmd/main.go

build:
	@echo "Building static binary..."
	CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o $(DIST_FOLDER)/$(BINARY_NAME) ./cmd/main.go

docker:
	@echo "Building dockerfile..."
	docker build -t retreat-roster .

docker-push:
	@echo "Pushing docker image..."
	docker tag retreat-roster docker.beaudan.me/retreat-roster
	docker push docker.beaudan.me/retreat-roster:latest

.PHONY: clean

clean:
	@echo "Cleaning up..."
	rm -rf $(DIST_FOLDER)/$(BINARY_NAME)
