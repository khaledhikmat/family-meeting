Please note the following about this project:

- This is an attempt to implement SFU (Selective Forwarding Unit) to support meeting broadcasts. There are some solutions such as [mediasoup](https://mediasoup.org/) that provides WebRTC Video Conferencing. Here is a simple project that demos mediasoup: [https://github.com/mkhahani/mediasoup-sample-app/tree/master](https://github.com/mkhahani/mediasoup-sample-app/tree/master).
- There are two modes: `Monitor` and `Broadcast`. `Monitor` is responsible to detect an initial offer from a broadcaster and delegate requests (via a pub/sub topic) to `Broadcast`.
- The `Broadcast` launches a separate asynchronous process to deal with each broadcast request. The broadcast manager always answers offers...it never initiates an offer.
- Each connected `Participant` runs asynchronously to establish a particular connection using WebRTC offer/answer negotiation. 
- The backend of this is Firestore to track calls, offers and answers. 
- Each remote WebRTC stream is managed by a broadcast manager. 

*There is a more difficult variation to this approach which is to allow the monitor to connect to the stream of IP-based cameras using RTSP and convert to WebRTC. Once it is in WebRTC, it can be broadcasted to connected WebRTC browsers. There are some solutions based on [Pion](github.com/pion/webrtc/v4): [RTSPtoWeb](https://github.com/deepch/RTSPtoWeb).*

## Go Module

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

## Env Variables

| NAME           | DEFAULT | DESCRIPTION       |
|----------------|-----|------------------|
| APP_NAME       | `none`  | Name of the microservice to appear in OTEL. |
| APP_PORT       | `8080`  | HTTP Server port. Required to expose API Endpoints. |
| GOOGLE_CLOUD_PROJECT     | `family-meeting-aa853`  | Google Cloud project name.   |
| GOOGLE_APPLICATION_CREDENTIALS     | `/local/dir`  | Provides Service Account credentials for the Google project.   |
| DISABLE_TELEMETRY     | `false`  | If `true`, it disables collecting OTEL telemetry signals.   |
| OTEL_EXPORTER_OTLP_ENDPOINT     | `http://localhost:4318`  | OTEL endpoint.   |
| OTEL_SERVICE_NAME     | `family-meeting-core`  | OTEL application name.   |
| OTEL_GO_X_EXEMPLAR     | `true`  | OTEL GO.   |
| EXPERIMENT_RTP_SEP_RW  | 'false`  | If `true`, it experiments with sending RTP packets through a local channel.  |
| RUN_TIME_ENV  | 'dev`  | Runetime env name.  |

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

## Run Core Locally

### Standalone

*This expects that the Google Collector be running or that the OTEL collection be disabled.*

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

## Run Web Locally

Please refer to the [web README](../web/README.md) to see how you can start the web locally.

## Things to do

- Security rules for database.
- Genkit in Go.
- Google OTEL.
    - Still unable to see metrics properly.
    - localhost:4318 is not reachable.
- Firebase deployment from CLI.
- Firebase deployment from CICD.
- Firebase collection delete docs.
- GKS (Autopilot) deployment from Terraform.
    - Firebase databse.
    - Pubsub Topic.
    - 2 services: monitor and broadcast.
    - How would I provide an authentication to pubsub and firebase from a GKS workload? They have something called [workload-identity](https://cloud.google.com/kubernetes-engine/docs/how-to/workload-identity)

- Abstract PubSub.
- Disallow more than 3 broadcasts per instance.
- Increase the UDP buffer so we don't lose data!


