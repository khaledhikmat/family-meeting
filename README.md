This is a family meeting room to allow us to speak using peer-2-peer streaming protocol i.e. WebRTC in two modes:
- Conversation Mode: two particpants.
- Presentation Mode: one presenter and multiple particpants.

This project consists of two main repos:
1. `Web` is a Firebase front-end project that also contains the Firestore database. Here are some notes:

- I was not able to install `firebase-tools` globally due to permission issue. So I created a folder and installed locally.
- Used `firebase init` to start a project using `hosting` only.
- I enabled the experimentation `webframework` feature so I can use `vite` using vanilla Javascript.
- `firebase` npm package must be installed locally within the `hosting` folder.
- `firebase login` to login to Google account.
- Deployment using `firebase deploy` from the `hosting` folder.
- The database must be protected. Currently it is set for all reads and writes in test mode.
- I also enabled the emulator.
- Protected the project setting using `vite` env vars.
- `npm run dev` to run locally
- `firebase deploy` to deploy

1. `Core` is a Go server-side code.

- Implement SFU (Selective Forwarding Unit) to support presentation meeting rooms. There are some solutions such as [mediasoup](https://mediasoup.org/) that provides WebRTC Video Conferencing. Here is a simple project that demos mediasoup: [https://github.com/mkhahani/mediasoup-sample-app/tree/master](https://github.com/mkhahani/mediasoup-sample-app/tree/master).
- Each remote WebRTC stream will be managed by a server agent. 
- The agent always answers offers...it never initiates an offer.
- Meeting manager has visibility on all agents. Challenge: the meeting manager must be on the same server as the agents. Otherwise the connection will be quite slow. This means that meetings must be conducted on the same server.
- How do we convert RTSP to WebRTC? This is to handle IP-based cameras. There are some solutions based on [Pion](github.com/pion/webrtc/v4): [RTSPtoWeb](https://github.com/deepch/RTSPtoWeb).
- The backend of this will be Firestore to track calls, offers and answers. Both the particpant browsers and the Media server will be connected to the same Firebase database. 
- The media server will be a headless server written in Go and deployed on GCP as GKS Service and able to communicate with a Firebase database to receive WebRTC offers and respond with answers. This way it establishes WebRTC link with each WebRTC stream.  

## To Do

*Please note that Google Cloud Run is not applicable to run a payload like `monitor` and `broadcast` services. This is because Cloud Run is a serverless service and meant to serve API Endpoints only or Job. Background headless processes do not work in Cloud Run env because the platform might evict the service if it has not received any API access.*

*Given this, I must admit that, so far, I have not seen better than Azure Container Apps where these headless workloads can run well with ease. I say ease because there is no need to setup K8s clusters. The alternative to Google Cloud Run within GCP is a GKS cluster which is a huge undertaking (opertationl and cost).*

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






