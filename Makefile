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

docker-update: build
	@echo "Building dockerfile..."
	sudo docker compose up --build -d

docker-run: build
	@echo "Building dockerfile..."
	docker-compose build
	docker-compose up

docker-push: build
	@echo "Pushing docker image..."
	docker build -t retreat-roster .
	docker tag retreat-roster docker.beaudan.me/retreat-roster
	docker push docker.beaudan.me/retreat-roster:latest

refresh-db:
	rm -rf ./.devenv/state/mongodb.bak
	mv -f ./.devenv/state/mongodb ./.devenv/state/mongodb.bak
	rsync -r --progress --exclude "journal/" --exclude "diagnostic.data" retreat:~/roster/db_bak/ ./.devenv/state/mongodb

.PHONY: clean

clean:
	@echo "Cleaning up..."
	rm -rf $(DIST_FOLDER)/$(BINARY_NAME)
