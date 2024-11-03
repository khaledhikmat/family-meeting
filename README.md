This is a family meeting room to allow us to speak using peer-2-peer streaming protocol i.e. WebRTC in two modes:
- Conversation Mode: two particpants.
- Presentation Mode: one presenter and multiple particpants.

This project consists of three main parts:
1. `Web` is a Firebase front-end project that also contains the Firestore database. Please refer to the [web README](./web/README.md).

1. `Core` is a headless server written in Go and deployed on GCP as GKS Service and able to communicate with a Firestore database to receive WebRTC offers and respond with answers. Please refer to the [core README](./core/README.md).  

1. `Deployment` is a Terraform project to deploy core to GCP GKS. Please refer to the [deployment README](./deployment/README.md).  

*Please note that Google Cloud Run is not applicable to run a payload like `monitor` and `broadcast` services. This is because Cloud Run is a serverless service and meant to serve API Endpoints only or Job. Background headless processes do not work in Cloud Run env because the platform might evict the service if it has not received any API access.*

*Given this, I must admit that, so far, I have not seen better than Azure Container Apps where these headless workloads can run well with ease. I say ease because there is no need to setup K8s clusters. The alternative to Google Cloud Run within GCP is a GKS cluster which is a huge undertaking (opertationl and cost).*






