import './style.css'
// Firebase imports
import { initializeApp } from 'firebase/app'
import { getAuth, signOut, signInWithPopup, GoogleAuthProvider } from "firebase/auth";
import { getFirestore, serverTimestamp, collection, doc, addDoc, setDoc, getDoc, getDocs, updateDoc, query, orderBy, limit, onSnapshot  } from "firebase/firestore";

// Firebase configuration
const firebaseConfig = {
  apiKey: import.meta.env.VITE_FIREBASE_API_KEY,
  authDomain: import.meta.env.VITE_FIREBASE_AUTH_DOMAIN,
  projectId: import.meta.env.VITE_FIREBASE_PROJECT_ID,
  storageBucket: import.meta.env.VITE_FIREBASE_STORAGE_BUCKET,
  messagingSenderId: import.meta.env.VITE_FIREBASE_MESSAGING_SENDER_ID,
  appId: import.meta.env.VITE_FIREBASE_APP_ID,
  measurementId: import.meta.env.VITE_FIREBASE_MEASUREMENT_ID
};

// Firebase initialization
const app = initializeApp(firebaseConfig);

// Firebase authentication
const provider = new GoogleAuthProvider();
const auth = getAuth();

const whenSignedIn = document.getElementById('whenSignedIn');
const whenSignedOut = document.getElementById('whenSignedOut');

const signInBtn = document.getElementById('signInBtn');
const signOutBtn = document.getElementById('signOutBtn');

const userDetails = document.getElementById('userDetails');

// Sign in event handlers
let signedUsername = '';

signInBtn.onclick = () => signInWithPopup(auth, provider);

signOutBtn.onclick = () => signOut(auth);

auth.onAuthStateChanged(user => {
    if (user) {
        // signed in
        whenSignedIn.hidden = false;
        whenSignedOut.hidden = true;
        userDetails.innerHTML = `<h3>Hello ${user.displayName}!</h3> <p>User ID: ${user.uid}</p>`;
        signedUsername = user.displayName;
    } else {
        // not signed in
        whenSignedIn.hidden = true;
        whenSignedOut.hidden = false;
        userDetails.innerHTML = '';
    }
});

// Firebase firestore 
const db = getFirestore(app);

// WebRTC
// STUN servers
const servers = {
  iceServers: [
    {
      urls: ['stun:stun1.l.google.com:19302', 'stun:stun2.l.google.com:19302'],
    },
  ],
  iceCandidatePoolSize: 10,
};

let log = msg => {
  document.getElementById('logs').innerHTML += new Date() + msg + '<br>';
}

// HTML elements
const remoteVideo = document.getElementById('remoteVideo');
const joinButton = document.getElementById('joinButton');
const broadcastInput = document.getElementById('broadcastInput');

// Global State
let pc = null;
let remoteStream = null;

joinButton.onclick = async () => {
  pc = new RTCPeerConnection(servers);
  pc.onicecandidatestatechange = (event) => {
    log('ice candidate state change', pc.iceConnectionState);
  };

  pc.onicecandidate = (event) => {
    // log('ice candidate received');
    // if (event.candidate === null) {
    //   log('ice candidate gathering incomplete');
    // } else {
    //   log('ice candidate gathering complete');
    // }
  };

  pc.ontrack = (event) => {
    log('ontrack');
    if (event.streams && event.streams[0]) {
      remoteStream = event.streams[0];
      log(`ontrack - remote locked ${remoteStream}`);
      remoteVideo.srcObject = remoteStream;
      remoteVideo.autoplay = true
      remoteVideo.controls = true
    }
  };

  pc.addTransceiver('video', { direction: 'recvonly' });

  // Create offer
  const offerDescription = await pc.createOffer();
  await pc.setLocalDescription(offerDescription);
  log("offer created");

  // Reference Firestore collections for signaling
  const requestsRef = collection(db, "broadcast_requests");
  const requestDoc = doc(requestsRef);

  await setDoc(requestDoc, { 
    requestor: signedUsername,
    kind: 'participant',
    abort: false,
    answer: '',
    parent: broadcastInput.value,
    offer: btoa(JSON.stringify(pc.localDescription))
  });

  // Listen for remote answer
  onSnapshot(requestDoc, (snapshot) => {
    const data = snapshot.data();
    if (!pc.currentRemoteDescription && data?.answer) {
      log('participant remote peer answer received');
      const answerDescription = new RTCSessionDescription(JSON.parse(atob(data.answer)));
      pc.setRemoteDescription(answerDescription);
    }
  });

  joinButton.disabled = false;
  broadcastInput.value = '';
};

joinButton.disabled = false;
broadcastInput.value = '';
