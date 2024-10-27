```bash
go mod init github.com/khaledhikmat/family-meeting
go get -u github.com/pion/webrtc/v4
go get -u firebase.google.com/go/v4@latest
go get -u cloud.google.com/go/pubsub
go get -u github.com/mdobak/go-xerrors
go get -u github.com/fatih/color
go get -u go.opentelemetry.io/otel
go get -u go.opentelemetry.io/contrib/exporters/autoexport
go get -u go.opentelemetry.io/contrib/propagators/autoprop
go get -u github.com/gin-gonic/gin
```

## Setup Roles

In order to get access to pub/sub, we must [install the gloud CLI](https://cloud.google.com/sdk/docs/install-sdk) and add the pubsub role to the service account:

- Determine the IAM policies for a specific project:

```bash
gcloud projects get-iam-policy family-meeting-aa853
```

- Add PubSub role to an existing service account:

```bash
gcloud projects add-iam-policy-binding family-meeting-aa853 --member="serviceAccount:firebase-adminsdk-7ne7s@family-meeting-aa853.iam.gserviceaccount.com" --role="roles/pubsub.editor"
```

In order to get allow writing to trace and metrics, the following roles are required:

- Add Traces role to an existing service account:

```bash
gcloud projects add-iam-policy-binding family-meeting-aa853 
--member="serviceAccount:firebase-adminsdk-7ne7s@family-meeting-aa853.iam.gserviceaccount.com" --role="roles/cloudtrace.agent"
```

- Add Metrics role to an existing service account:

```bash
gcloud projects add-iam-policy-binding family-meeting-aa853 --member="serviceAccount:firebase-adminsdk-7ne7s@family-meeting-aa853.iam.gserviceaccount.com" --role="roles/monitoring.metricWriter"
```

## Build and Push to Docker Hub

```bash
make push-2-hub
```

## Run Google Collector

```bash
docker container run --platform linux/amd64 -it -p 8080:8080 \
-e "APP_PORT=8080" \
-e "OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318" \
-e "OTEL_SERVICE_NAME=family-meeting-core" \
-e "OTEL_GO_X_EXEMPLAR=true" \
-e "GOOGLE_CLOUD_PROJECT=family-meeting-aa853" \
-e "GOOGLE_APPLICATION_CREDENTIALS=/Users/khaled/gcp-creds/family-meeting-service-account-key.json" \
-v "/Users/khaled/gcp-creds/family-meeting-service-account-key.json":"/Users/khaled/gcp-creds/family-meeting-service-account-key.json" \
--name fm-monitor \
khaledhikmat/family-meeting-core:latest
```

## Run Locally

### Standalone

*This expects that the Google Collector be running*

- Setup Credentials on each terminal below:

```bash
export GOOGLE_CLOUD_PROJECT="family-meeting-aa853"
export GOOGLE_APPLICATION_CREDENTIALS=/Users/khaled/gcp-creds/family-meeting-service-account-key.json
```

- Run Monitor in a terminal session:

```bash
go run main.go monitor
```

cntrl-c to stop

- Run Broadcast in a second terminal session:

```bash
go run main.go broadcast
```

cntrl-c to stop

### DAPR

DAPR CLI allows us to run just like Docker compose but without the need for images:

- Start Google OTEL Collector

```bash
make start-goocollector
```

- Start

```bash
make start
```

- Stop

```bash
./stop.sh
```

- Stop Google OTEL Collector

```bash
make stop-goocollector
```

### Docker Compose

Docker compose mimicks GKS environment locally but does require that images be built:

- Start 

```bash
# force a pull and run detached
docker compose up -d --pull always
# force a pull and run interactively
docker compose up --pull always
```

- Stop

Either control-c or :

```bash
docker compose down
```

