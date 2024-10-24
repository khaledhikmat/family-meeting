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
- The media server will be a headless server written in Go and deployed on GCP as Cloud Run and able to communicate with a Firebase database to receive WebRTC offers and respond with answers. This way it establishes WebRTC link with each WebRTC stream.  

## To Do

- Github repository.
- Security rules for database.
- Connect to Firestore from Go.
- Install gcloud CLI.
- Genkit in Go.
- Google OTEL.






