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

signInBtn.onclick = () => signInWithPopup(auth, provider);

signOutBtn.onclick = () => signOut(auth);

auth.onAuthStateChanged(user => {
    if (user) {
        // signed in
        whenSignedIn.hidden = false;
        whenSignedOut.hidden = true;
        userDetails.innerHTML = `<h3>Hello ${user.displayName}!</h3> <p>User ID: ${user.uid}</p>`;
    } else {
        // not signed in
        whenSignedIn.hidden = true;
        whenSignedOut.hidden = false;
        userDetails.innerHTML = '';
    }
});

// Firebase firestore 

const db = getFirestore(app);

const createThing = document.getElementById('createThing');
const thingsList = document.getElementById('thingsList');


let thingsRef;
let unsubscribe;

auth.onAuthStateChanged(user => {

    if (user) {

        // Database Reference
        // thingsRef = collection(db, "things");
        // const q = query(thingsRef, orderBy('createdAt', 'desc'), limit(3));

        // createThing.onclick = async () => {
        //   console.log('createThing clicked');
        //   const docRef = await addDoc(thingsRef, {
        //     name: faker.commerce.productName(),
        //     weight: faker.datatype.number({ min: 5, max: 100 }),
        //     createdAt: serverTimestamp()
        //   });
        //   console.log("Document written with ID: ", docRef.id);
        //   // Query
        //   const querySnapshot = await getDocs(q);
        //   querySnapshot.forEach((doc) => {
        //       console.log(`${doc.id} => ${doc.data()}`);
        //   });
        // }

        // Realtime listener
        // unsubscribe = onSnapshot(q, querySnapshot => {
        //       // Map results to an array of li elements
        //       const items = querySnapshot.docs.map(doc => {
        //         return `<li>${doc.data().name}</li>`;
        //       });
        //       thingsList.innerHTML = items.join('');
        // });
    } else {
        // Unsubscribe when the user signs out
        // unsubscribe && unsubscribe();
        // createThing.onclick = null;
    }
});

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

// Global State
let pc = new RTCPeerConnection(servers);
let localStream = null;
let remoteStream = null;

// HTML elements
const webcamButton = document.getElementById('webcamButton');
const webcamVideo = document.getElementById('webcamVideo');
const callButton = document.getElementById('callButton');
const callInput = document.getElementById('callInput');
const answerButton = document.getElementById('answerButton');
const remoteVideo = document.getElementById('remoteVideo');
const hangupButton = document.getElementById('hangupButton');

// 1. Setup media sources

webcamButton.onclick = async () => {
  localStream = await navigator.mediaDevices.getUserMedia({ video: true, audio: true });
  remoteStream = new MediaStream();

  // Push tracks from local stream to peer connection
  localStream.getTracks().forEach((track) => {
    pc.addTrack(track, localStream);
  });

  // Pull tracks from remote stream, add to video stream
  pc.ontrack = (event) => {
    event.streams[0].getTracks().forEach((track) => {
      remoteStream.addTrack(track);
    });
  };

  webcamVideo.srcObject = localStream;
  remoteVideo.srcObject = remoteStream;

  callButton.disabled = false;
  answerButton.disabled = false;
  webcamButton.disabled = true;
};

// 2. Create an offer
callButton.onclick = async () => {
  // Reference Firestore collections for signaling
  const callsRef = collection(db, "calls");
  const callDoc = doc(callsRef);
  const offersRef = collection(callDoc, "offers");
  const answersRef = collection(callDoc, "answers");

  // TODO: Not needed. We can use the Ref directly.
  const offersQuery = query(offersRef);
  const answersQuery = query(answersRef);

  callInput.value = callDoc.id;

  // Get candidates for caller, save to db
  pc.onicecandidate = (event) => {
    console.log('caller ice candidate received');
    event.candidate && addDoc(offersRef, event.candidate.toJSON());
  };

  // Create offer
  const offerDescription = await pc.createOffer();
  await pc.setLocalDescription(offerDescription);

  const offer = {
    sdp: offerDescription.sdp,
    type: offerDescription.type,
  };

  await setDoc(callDoc, { offer });

  // Listen for remote answer
  onSnapshot(callDoc, (snapshot) => {
    const data = snapshot.data();
    if (!pc.currentRemoteDescription && data?.answer) {
      console.log('caller remote answer received');
      const answerDescription = new RTCSessionDescription(data.answer);
      pc.setRemoteDescription(answerDescription);
    }
  });

  // When answered, add candidate to peer connection
  onSnapshot(answersQuery, querySnapshot => {
    querySnapshot.docChanges().forEach((change) => {
      console.log('caller new answer candidate received', change);
      if (change.type === 'added') {
        console.log('caller new answer candidate received2');
        const candidate = new RTCIceCandidate(change.doc.data());
        pc.addIceCandidate(candidate);
      }
    });
  });

  hangupButton.disabled = false;
};

// 3. Answer the call with the unique ID
answerButton.onclick = async () => {
  const callId = callInput.value;
  const callsRef = collection(db, "calls");
  const callDoc = doc(callsRef, callId);
  const offersRef = collection(callDoc, "offers");
  const answersRef = collection(callDoc, "answers");

  // TODO: Not needed. We can use the Ref directly.
  const offersQuery = query(offersRef);
  const answersQuery = query(answersRef);

  pc.onicecandidate = (event) => {
    console.log('answerer ice candidate received');
    event.candidate && addDoc(answersRef, event.candidate.toJSON());
  };

  const callData = (await getDoc(callDoc)).data();

  const offerDescription = callData.offer;
  await pc.setRemoteDescription(new RTCSessionDescription(offerDescription));

  const answerDescription = await pc.createAnswer();
  await pc.setLocalDescription(answerDescription);

  const answer = {
    type: answerDescription.type,
    sdp: answerDescription.sdp,
  };

  await updateDoc(callDoc, { answer });

  onSnapshot(offersQuery, querySnapshot => {
    querySnapshot.docChanges().forEach((change) => {
      console.log('answerer remote offer received', change);
      if (change.type === 'added') {
        console.log('answerer remote offer received2');
        let data = change.doc.data();
        pc.addIceCandidate(new RTCIceCandidate(data));
      }
    });
  });
};

// 4. Hangup
hangupButton.onclick = () => {
  // Close the RTCPeerConnection
  if (pc) {
    pc.close();
    pc = null;
  }

  // Reset UI elements
  callButton.disabled = false;
  answerButton.disabled = false;
  hangupButton.disabled = true;
  callInput.value = '';

  console.log('Call ended.');
};
