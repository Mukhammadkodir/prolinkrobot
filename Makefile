CURRENT_DIR=$(shell pwd)

APP=$(shell basename ${CURRENT_DIR})
APP_CMD_DIR=${CURRENT_DIR}/cmd

TAG=latest
ENV_TAG=latest

pull-proto-module:
	git submodule update --init --recursive

update-proto-module:
	git submodule update --remote --merge

copy-proto-module:
	rm -rf ${CURRENT_DIR}/protos
	rsync -rv --exclude=.git ${CURRENT_DIR}/mb_protos/* ${CURRENT_DIR}/protos

gen-proto-module:
	./scripts/gen_proto.sh ${CURRENT_DIR}

rm-proto-omit-empty:
	chmod 744 ./scripts/rm_omit_empty.sh && ./scripts/rm_omit_empty.sh ${CURRENT_DIR}

build:
	go build -mod=vendor -a -installsuffix cgo -o ${CURRENT_DIR}/bin/${APP} ${APP_CMD_DIR}/main.go

build-image:
	docker build --rm -t ${REGISTRY}/${PROJECT_NAME}/${APP}:${TAG} .
	docker tag ${REGISTRY}/${PROJECT_NAME}/${APP}:${TAG} ${REGISTRY}/${PROJECT_NAME}/${APP}:${ENV_TAG}

push-image:
	docker push ${REGISTRY}/${PROJECT_NAME}/${APP}:${TAG}
	docker push ${REGISTRY}/${PROJECT_NAME}/${APP}:${ENV_TAG}

swag-init:
	swag init -g api/api.go -o api/docs

run:
	go run cmd/main.go

build-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=$${TARGET_ARCH:-amd64} go build -o ./dist/prolinkrobot ./cmd

package-lightsail:
	./scripts/package_lightsail.sh

deploy-lightsail:
	./scripts/deploy_lightsail.sh

linter:
	golangci-lint run

	migrate -path ./migrations -database 'postgres://husniddinnurmamatov:@localhost:5432/ntm_go_service?sslmode=disable' up
migration-up:
	migrate -path ./migrations -database 'postgres://husniddinnurmamatov:@localhost:5432/ntm_go_service?sslmode=disable' up

migration-down:
	migrate -path ./migrations -database 'postgres://husniddinnurmamatov:@localhost:5432/ntm_go_service?sslmode=disable' down

# git pull origin main --no-rebase
