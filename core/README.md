```bash
go mod init github.com/khaledhikmat/family-meeting
go get -u github.com/pion/webrtc/v4
go get -u firebase.google.com/go/v4@latest
go get -u cloud.google.com/go/pubsub
```

## Local Development Env

```bash
export GOOGLE_APPLICATION_CREDENTIALS=/Users/khaled/gcp-creds/family-meeting-service-account-key.json
```

## PubSub

In order to get access to pub/sub, we must [install the gloud CLI](https://cloud.google.com/sdk/docs/install-sdk) and add the pubsub role to the service account:

- Determine the IAM policies for a specific project:

```bash
gcloud projects get-iam-policy family-meeting-aa853
```

- Add PubSub role to an existing service account:

```bash
gcloud projects add-iam-policy-binding family-meeting-aa853 --member="serviceAccount:firebase-adminsdk-7ne7s@family-meeting-aa853.iam.gserviceaccount.com" --role="roles/pubsub.editor"
```

