BINARY_NAME=main
DIST_FOLDER=./dist

.PHONY: build

run:
	go run cmd/main.go

cleandb:
	rm -rf .devenv/state/mongodb
	mv data/state* data/state.json

build:
	@echo "Building static binary..."
	tailwindcss -i ./www/input.css -o ./www/app.css
	CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o $(DIST_FOLDER)/$(BINARY_NAME) ./cmd/main.go

docker-build:
	@echo "Building dockerfile..."
	docker-compose build

docker-run: build
	@echo "Building dockerfile..."
	docker-compose build
	docker-compose up

docker-push: build
	@echo "Pushing docker image..."
	docker build -t retreat-roster .
	docker tag retreat-roster docker.beaudan.me/retreat-roster
	docker push docker.beaudan.me/retreat-roster:latest

.PHONY: clean

clean:
	@echo "Cleaning up..."
	rm -rf $(DIST_FOLDER)/$(BINARY_NAME)
