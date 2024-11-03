Please note the following about this project:

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

## Run Web Locally

- Make sure that the Core services are running as in [core README](../core/README.md).

- Run local HTTP server in a terminal session:

```bash
cd web
npm run dev
```

- In one browser session, access [http://localhost:5173/index.html](http://localhost:5173/index.html) and select `broadcast` and click the `start` broadcast. This produces a broadcast ID that you can copy from the text box.

*Please note that there is a 30-sec delay between starting a broadcast and accepting broadcast connections. This is to make sure that the connection with the broadcaster is stable and that we are not getting multiple `onRemoteTrack` connections.*

- In another browser session, access [http://localhost:5173/index.html](http://localhost:5173/index.html), paste the broadcast ID and select `join a broadcast`.

- You can repeat the above process several times in several tabs to simulate a true broadcast.

