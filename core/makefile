BUILD_DIR="./build"
DIST_DIR="./dist"

clean_build:
	if [ -d "${BUILD_DIR}" ]; then rm -r ${BUILD_DIR}; fi

clean_dist:
	if [ -d "${DIST_DIR}" ]; then rm -r ${DIST_DIR}; fi; mkdir ${DIST_DIR}

test:
	echo "Invoking test cases..."

build: clean_dist clean_build test
	GOOS='linux' GOARCH='amd64' GO111MODULE='on' go build -o "${BUILD_DIR}/family-meeting-core" .

# docker images | grep 'family-meeting' | awk '{print $3}' | xargs docker rmi -f
# docker ps -a | grep 'family-meeting' | awk '{print $1}' | xargs docker rm -f
# docker system prune -a
dockerize: clean_dist clean_build test build
	docker buildx build --platform linux/amd64 -t khaledhikmat/family-meeting-core:latest .
	docker buildx build --platform linux/amd64 -t khaledhikmat/family-meeting-collector:latest ./telemetry -f ./telemetry/Dockerfile

push-2-hub: clean_dist clean_build test build dockerize
	docker login
	docker push khaledhikmat/family-meeting-core:latest
	docker push khaledhikmat/family-meeting-collector:latest

start: clean_dist clean_build test
	dapr run -f .

list: 
	dapr list

stop: 
	#./stop.sh
	dapr stop -f . && (lsof -i:8080 | grep main) | awk '{print $2}' | xargs kill && (lsof -i:8081 | grep main) | awk '{print $2}' | xargs kill && (lsof -i:8082 | grep main) | awk '{print $2}' | xargs kill && (lsof -i:8083 | grep main) | awk '{print $2}' | xargs kill && (lsof -i:3000 | grep main) | awk '{print $2}' | xargs kill

# The double $$ is used to escape the $ sign in the makefile
start-goocollector:
	export USERIDGOOGLE_CLOUD_PROJECT="family-meeting-aa853" && \
	export GOOGLE_APPLICATION_CREDENTIALS="/Users/khaled/gcp-creds/family-meeting-service-account-key.json" && \
	docker run -d --rm -p 4318:4318 \
			-e "GOOGLE_CLOUD_PROJECT=${GOOGLE_CLOUD_PROJECT}" \
			-e "GOOGLE_APPLICATION_CREDENTIALS=${GOOGLE_APPLICATION_CREDENTIALS}" \
            -v "${PWD}/telemetry/otel-collector-config.yaml":/etc/otelcol-contrib/config.yaml \
            -v "/Users/khaled/gcp-creds/family-meeting-service-account-key.json":"$$HOME/.config/gcloud/application_default_credentials.json" \
            --name goocollector otel/opentelemetry-collector-contrib:0.105.0 \
            --config /etc/otelcol-contrib/config.yaml; \

stop-goocollector:
	docker stop goocollector

compose-up:
	# force a pull and run detached
	# for some reason, running from here does not run due to amd64 platform
	docker compose up -d --pull always

compose-down:
	docker compose down
