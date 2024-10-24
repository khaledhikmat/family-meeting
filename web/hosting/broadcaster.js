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
const localVideo = document.getElementById('localVideo');
const startButton = document.getElementById('startButton');
const broadcastInput = document.getElementById('broadcastInput');
const hangupButton = document.getElementById('hangupButton');

// Global State
let pc = null;
let localStream = null;

let setInitialState = async () => {
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

  // Setup media sources
  localStream = await navigator.mediaDevices.getUserMedia({ video: true, audio: true });

  // Push tracks from local stream to peer connection
  localStream.getTracks().forEach((track) => {
    pc.addTrack(track, localStream);
  });

  localVideo.srcObject = localStream;

  startButton.disabled = false;
  hangupButton.disabled = true;
  broadcastInput.value = '';
};

// Handle start button
startButton.onclick = async () => {
  // Reference Firestore collections for signaling
  const requestsRef = collection(db, "broadcast_requests");
  const requestDoc = doc(requestsRef);
  broadcastInput.value = requestDoc.id;

  // Create offer
  const offerDescription = await pc.createOffer();
  await pc.setLocalDescription(offerDescription);
  log("offer created");

  await setDoc(requestDoc, { 
    requestor: signedUsername,
    kind: 'broadcaster',
    abort: false,
    answer: '',
    parent: '',
    offer: btoa(JSON.stringify(pc.localDescription))
  });

  // Listen for remote answer
  onSnapshot(requestDoc, (snapshot) => {
    const data = snapshot.data();
    if (!pc.currentRemoteDescription && data?.answer) {
      log('broadcaster remote peer answer received');
      const answerDescription = new RTCSessionDescription(JSON.parse(atob(data.answer)));
      pc.setRemoteDescription(answerDescription);
    }
  });

  startButton.disabled = true;
  hangupButton.disabled = false;
};

// 4. Hangup
hangupButton.onclick = async () => {
  const requestsRef = collection(db, "broadcast_requests");
  const requestDoc = doc(requestsRef, broadcastInput.value);
  await updateDoc(requestDoc, { abort: true });

  // Close the RTCPeerConnection
  if (pc) {
    pc.close();
    pc = null;
  }

  // Set initial state
  await setInitialState();
  log('Broadcast ended.');
};

// Set initial state
await setInitialState();


