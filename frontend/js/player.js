// js/player.js

import { request, resolvePath } from './api.js';
import {
  initializeCastApi,
  castPlayItem,
  isCasting,
  getRemotePlayer,
  setupCastCallbacks,
  getCastContext,
  getRemotePlayerController
} from './player/cast.js';

import {
  getQueue,
  getQueueLength,
  setQueue,
  queueShift,
  queueSome,
  registerNotificationCallback,
  updateQueueUI
} from './player/queue.js';

export { addToQueue } from './player/queue.js';


import {
  setPlayerController,
  updateTimelineUI,
  updatePlayPauseButton,
  updateVolumeIcon,
  updateMetadataUI,
  updateSkipButtonsUI,
  updatePlaybackControlsUI,
  showNotification,
  updateSleepTimerUI,
  setupUIEventListeners,
  drawWaveform
} from './player/ui.js';


let audio = null;
let hls = null;
let currentItem = null;
let currentPlaylistUri = null;
let progressInterval = null;
let isMuted = false;
let previousVolume = 1.0;
let currentWaveform = null;
let hoverPct = null;
let hoverX = null;
let playbackQueue = [];
let draggedQueueIndex = null;

// Sleep Timer variables
let sleepTimerId = null;
let sleepTimerDuration = 0;
let sleepTimerTimeRemaining = 0;
let sleepTimerEndTimestamp = 0;
let sleepTimerType = 'off';
let sleepTimerAutoRestart = false;
let sleepTimerShakeToReset = true;
let userVolume = 1.0;
let isFading = false;
let isSleepTimerActive = false;
let lastShakeTime = 0;

// Volume Boost and Playback Speed persistence variables
let audioContext = null;
let audioSourceNode = null;
let gainNode = null;
let volumeBoostLevel = 1.0; // multiplier: 1.0 = no boost, 1.5, 2.0, 2.5, 3.0
let rememberSpeedPerBook = true;
let globalDefaultSpeed = 1.0;
let playerSkipBackSeconds = 10;
let playerSkipForwardSeconds = 10;




// Initialize audio and elements
function initAudio() {
  if (audio) return;
  audio = new Audio();
  audio.crossOrigin = "anonymous";
  
  // Set up audio events for timeline and status updates
  audio.addEventListener('timeupdate', updateTimelineUI);
  audio.addEventListener('durationchange', updateTimelineUI);
  audio.addEventListener('ended', onPlaybackEnded);
  audio.addEventListener('play', () => {
    if (isCasting()) return;
    updatePlayPauseButton(true);
    if ('mediaSession' in navigator) {
      navigator.mediaSession.playbackState = 'playing';
    }
    
    // Auto-restart sleep timer if enabled
    if (sleepTimerAutoRestart && sleepTimerType !== 'off' && !isSleepTimerActive) {
      startSleepTimer(sleepTimerType);
    }
    
    document.dispatchEvent(new CustomEvent('playback-state-changed', {
      detail: { itemId: currentItem?.id, isPlaying: true }
    }));
  });
  audio.addEventListener('pause', () => {
    if (isCasting()) return;
    updatePlayPauseButton(false);
    if ('mediaSession' in navigator) {
      navigator.mediaSession.playbackState = 'paused';
    }
    
    // Clear/stop sleep timer on pause, but preserve sleepTimerType for auto-restart
    if (isSleepTimerActive) {
      stopSleepTimer(false); // false means don't clear the saved type
    }
    
    document.dispatchEvent(new CustomEvent('playback-state-changed', {
      detail: { itemId: currentItem?.id, isPlaying: false }
    }));
  });
  
  setupUIEventListeners();
}

function initVolumeBoost() {
  if (!audio) return;
  if (audioContext) return;
  
  try {
    const AudioContextClass = window.AudioContext || window.webkitAudioContext;
    if (!AudioContextClass) {
      console.warn('Web Audio API not supported');
      return;
    }
    audioContext = new AudioContextClass();
    audioSourceNode = audioContext.createMediaElementSource(audio);
    gainNode = audioContext.createGain();
    
    audioSourceNode.connect(gainNode);
    gainNode.connect(audioContext.destination);
    
    gainNode.gain.value = volumeBoostLevel;
    console.log('Web Audio volume boost initialized:', volumeBoostLevel);
  } catch (err) {
    console.error('Failed to initialize volume boost:', err);
  }
}

export async function playItems(items, startIndex = 0) {
  if (!items || items.length === 0) return;
  setQueue(items.slice(startIndex + 1));
  const itemToPlay = items[startIndex];
  updateQueueUI();
  await playItem(itemToPlay);
}

export function pauseItem() {
  if (isCasting()) {
    const session = getCastContext()?.getCurrentSession();
    if (session) {
      getRemotePlayerController()?.playOrPause();
    }
  } else {
    if (audio) {
      audio.pause();
    }
  }
}

export async function playItem(item, startTime = 0) {
  initAudio();
  if (volumeBoostLevel > 1.0) {
    initVolumeBoost();
  }
  if (audioContext && audioContext.state === 'suspended') {
    audioContext.resume();
  }
  
  let itemObj = item;
  if (typeof item === 'string') {
    try {
      itemObj = await request('GET', `/api/items/${item}`);
    } catch (err) {
      console.error('Failed to fetch item for playback:', err);
      return;
    }
  }
  
  try {
    // 1. Create playback session
    const playUrl = itemObj.episodeId 
      ? `/api/items/${itemObj.id}/play/${itemObj.episodeId}` 
      : `/api/items/${itemObj.id}/play`;
    const playResponse = await request('POST', playUrl, { startTime });
    const sessionId = playResponse.id;
    let clientPlaylistUri = playResponse.clientPlaylistUri;
    const token = localStorage.getItem('token');
    if (token && clientPlaylistUri) {
      if (clientPlaylistUri.includes('?')) {
        clientPlaylistUri += `&token=${token}`;
      } else {
        clientPlaylistUri += `?token=${token}`;
      }
    }
    
    currentItem = itemObj;
    currentPlaylistUri = clientPlaylistUri;
    fetchWaveform(itemObj.id);
    
    // Restore saved playback speed or default speed
    let speedToUse = globalDefaultSpeed;
    if (rememberSpeedPerBook) {
      const storedSpeed = localStorage.getItem(`abs-speed-book-${itemObj.id}`);
      if (storedSpeed) {
        speedToUse = parseFloat(storedSpeed);
      }
    }
    const speedSelectEl = document.getElementById('player-speed');
    if (speedSelectEl) {
      let optionExists = false;
      for (let i = 0; i < speedSelectEl.options.length; i++) {
        if (parseFloat(speedSelectEl.options[i].value) === speedToUse) {
          optionExists = true;
          break;
        }
      }
      if (!optionExists) {
        const customOpt = document.createElement('option');
        customOpt.value = speedToUse.toString();
        customOpt.textContent = `${speedToUse}x`;
        speedSelectEl.appendChild(customOpt);
      }
      speedSelectEl.value = speedToUse.toString();
    }
    
    // Stop any existing playback and progress reporting
    stopProgressReporting();
    if (hls) {
      hls.destroy();
      hls = null;
    }

    
    // Set metadata on UI
    updateMetadataUI(itemObj);
    
    if (isCasting()) {
      if (audio) {
        audio.pause();
        audio.src = '';
      }
      castPlayItem(itemObj, startTime, clientPlaylistUri);
    } else {
      // Check HLS support
      const isNativeHlsSupported = audio.canPlayType('application/vnd.apple.mpegurl') || 
                                   audio.canPlayType('application/x-mpegURL');
      
      if (isNativeHlsSupported) {
        audio.src = resolvePath(clientPlaylistUri);
        audio.currentTime = startTime;
        audio.play().catch(err => console.error('Playback start failed:', err));
      } else {
        await loadHlsScript();
        if (hls) {
          hls.destroy();
        }
        hls = new Hls();
        hls.loadSource(resolvePath(clientPlaylistUri));
        hls.attachMedia(audio);
        
        hls.on(Hls.Events.MANIFEST_PARSED, () => {
          audio.currentTime = startTime;
          audio.play().catch(err => console.error('Hls.js playback start failed:', err));
        });
        
        hls.on(Hls.Events.ERROR, (event, data) => {
          if (data.fatal) {
            switch (data.type) {
              case Hls.ErrorTypes.NETWORK_ERROR:
                console.error('Fatal network error, trying to recover...', data);
                hls.startLoad();
                break;
              case Hls.ErrorTypes.MEDIA_ERROR:
                console.error('Fatal media error, trying to recover...', data);
                hls.recoverMediaError();
                break;
              default:
                console.error('Unrecoverable Hls.js error:', data);
                destroyPlayer();
                break;
            }
          }
        });
      }
      
      // Apply speed and volume from controls
      const speedSelect = document.getElementById('player-speed');
      if (speedSelect) {
        audio.playbackRate = parseFloat(speedSelect.value) || 1.0;
      }
      
      const volumeSlider = document.getElementById('player-volume-slider');
      if (volumeSlider) {
        userVolume = parseFloat(volumeSlider.value) / 100;
        audio.volume = userVolume;
      }

    }
    
    // Show player bar
    const playerBar = document.getElementById('player-bar');
    if (playerBar) {
      playerBar.classList.remove('hidden');
    }
    
    // Start progress reporting
    startProgressReporting();
    
  } catch (err) {
    console.error('Failed to play item:', err);
    alert('Failed to initialize playback session.');
  }
}

// Progress reporting (every 10s)
function startProgressReporting() {
  stopProgressReporting();
  progressInterval = setInterval(() => {
    reportProgress(false);
  }, 10000);
}

function stopProgressReporting() {
  if (progressInterval) {
    clearInterval(progressInterval);
    progressInterval = null;
  }
}

async function reportProgress(isFinished = false) {
  if (!currentItem) return;
  
  let currentTime = 0;
  let duration = 0;
  if (isCasting()) {
    const rp = getRemotePlayer();
    currentTime = rp.currentTime || 0;
    duration = rp.duration || 0;
  } else {
    if (!audio) return;
    currentTime = audio.currentTime || 0;
    duration = audio.duration || 0;
  }
  
  const payload = {
    currentTime: currentTime,
    duration: duration,
    isFinished: isFinished
  };
  
  try {
    const progressUrl = currentItem.episodeId 
      ? `/api/me/progress/${currentItem.id}/${currentItem.episodeId}` 
      : `/api/me/progress/${currentItem.id}`;
    await request('PATCH', progressUrl, payload);
  } catch (err) {
    console.warn('Failed to save playback progress:', err);
  }
}

// Setup Event Listeners for Player UI Buttons/Sliders


// Format duration/time helper

// Update UI Helpers

async function onPlaybackEnded() {
  await reportProgress(true);
  if (getQueueLength() > 0) {
    const nextItem = queueShift();
    updateQueueUI();
    stopProgressReporting();
    if (hls) {
      hls.destroy();
      hls = null;
    }
    playItem(nextItem, 0);
  } else {
    destroyPlayer();
  }
}

export function seekTo(seconds) {
  if (isCasting()) {
    getRemotePlayer().currentTime = seconds;
    getRemotePlayerController().seek();
  } else {
    if (!audio) return;
    audio.currentTime = seconds;
  }
  updateTimelineUI();
}

function destroyPlayer() {
  stopProgressReporting();
  stopSleepTimer(true);
  window.removeEventListener('devicemotion', handleDeviceMotion);
  if (audio) {
    audio.pause();
    audio.src = '';
  }

  if ('mediaSession' in navigator) {
    navigator.mediaSession.playbackState = 'none';
  }

  if (hls) {
    hls.destroy();
    hls = null;
  }
  if (isCasting() && getCastContext()) {
    const session = getCastContext().getCurrentSession();
    if (session) {
      session.endSession(true);
    }
  }
  currentItem = null;
  currentPlaylistUri = null;
  
  document.dispatchEvent(new CustomEvent('playback-state-changed', {
    detail: { itemId: null, isPlaying: false }
  }));
  
  const playerBar = document.getElementById('player-bar');
  if (playerBar) {
    playerBar.classList.add('hidden');
  }

  const expandedDialog = document.getElementById('expanded-player-dialog');
  if (expandedDialog) {
    expandedDialog.close();
    expandedDialog.remove();
  }
}

// CDN script loader for hls.js
function loadHlsScript() {
  return new Promise((resolve, reject) => {
    if (window.Hls) {
      resolve();
      return;
    }
    const script = document.createElement('script');
    script.src = 'https://cdn.jsdelivr.net/npm/hls.js@1.5.7';
    script.onload = () => {
      if (window.Hls) {
        resolve();
      } else {
        reject(new Error('Hls.js failed to load'));
      }
    };
    script.onerror = () => reject(new Error('Failed to load Hls.js script'));
    document.head.appendChild(script);
  });
}

function escapeHtml(str) {
  if (!str) return '';
  return str.toString()
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&#039;");
}

export function getCurrentPlayingItem() {
  return currentItem;
}

export function getCurrentPlaybackTime() {
  if (isCasting()) {
    return getRemotePlayer().currentTime || 0;
  }
  return audio ? audio.currentTime : 0;
}

// ==========================================
// Sleep Timer Implementation
// ==========================================

function startSleepTimer(type) {
  stopSleepTimer(false); // Clear any existing active timer first
  
  sleepTimerType = type;
  if (type === 'off') {
    updateSleepTimerUI();
    saveSleepTimerSettings();
    return;
  }
  
  let durationInSeconds = 0;
  if (type === 'chapter') {
    // Find remaining time in current chapter
    const currentSecs = audio ? audio.currentTime : 0;
    const chapters = currentItem?.media?.chapters || [];
    const activeChapter = chapters.find(c => currentSecs >= c.start && currentSecs < c.end);
    if (activeChapter) {
      durationInSeconds = Math.max(activeChapter.end - currentSecs, 0);
    } else {
      durationInSeconds = 0;
    }
  } else {
    durationInSeconds = parseInt(type, 10) * 60;
  }
  
  if (durationInSeconds <= 0) {
    console.warn('Sleep timer duration is 0 or negative');
    return;
  }
  
  isSleepTimerActive = true;
  sleepTimerTimeRemaining = durationInSeconds;
  sleepTimerEndTimestamp = Date.now() + durationInSeconds * 1000;
  
  if (audio) {
    userVolume = audio.volume;
  }
  isFading = false;
  
  sleepTimerId = setInterval(tickSleepTimer, 1000);
  
  updateSleepTimerUI();
  saveSleepTimerSettings();
  
  if (sleepTimerShakeToReset) {
    enableShakeDetection();
  }
}

function tickSleepTimer() {
  if (!isSleepTimerActive || !audio || audio.paused) {
    stopSleepTimer(false);
    return;
  }
  
  const now = Date.now();
  const remaining = Math.max(Math.round((sleepTimerEndTimestamp - now) / 1000), 0);
  sleepTimerTimeRemaining = remaining;
  
  // Update badge UI
  const text = remaining > 60 ? `${Math.ceil(remaining / 60)}m` : `${remaining}s`;
  ['player-sleep-badge', 'expanded-sleep-badge'].forEach(id => {
    const badge = document.getElementById(id);
    if (badge) {
      badge.classList.remove('hidden');
      badge.textContent = text;
    }
  });
  
  // Fade-out handling (last 30 seconds)
  if (remaining <= 30) {
    if (!isFading) {
      isFading = true;
      console.log('Sleep timer starting volume fade-out');
    }
    const fadeFactor = remaining / 30; // goes from 1.0 down to 0.0
    audio.volume = userVolume * fadeFactor;
  } else {
    if (isFading) {
      isFading = false;
      audio.volume = userVolume;
    }
  }
  
  if (remaining <= 0) {
    console.log('Sleep timer expired. Pausing playback.');
    audio.pause();
    stopSleepTimer(false);
    if (audio) {
      audio.volume = userVolume;
    }
  }
}

function stopSleepTimer(clearType = true) {
  if (sleepTimerId) {
    clearInterval(sleepTimerId);
    sleepTimerId = null;
  }
  isSleepTimerActive = false;
  
  if (audio && isFading) {
    audio.volume = userVolume;
  }
  isFading = false;
  
  if (clearType) {
    sleepTimerType = 'off';
  }
  
  ['player-sleep-badge', 'expanded-sleep-badge'].forEach(id => {
    const badge = document.getElementById(id);
    if (badge) {
      badge.classList.add('hidden');
    }
  });
  
  ['player-sleep-icon', 'expanded-sleep-icon'].forEach(id => {
    const sleepIcon = document.getElementById(id);
    if (sleepIcon) {
      sleepIcon.classList.remove('text-accent');
    }
  });
  
  saveSleepTimerSettings();
}

function resetSleepTimer() {
  if (!isSleepTimerActive) return;
  console.log('Resetting sleep timer');
  
  startSleepTimer(sleepTimerType);
  
  const sleepIcon = document.getElementById('player-sleep-icon');
  if (sleepIcon) {
    sleepIcon.classList.add('animate-bounce');
    setTimeout(() => {
      sleepIcon.classList.remove('animate-bounce');
    }, 1000);
  }
}

function saveSleepTimerSettings() {
  const settings = {
    autoRestart: sleepTimerAutoRestart,
    shakeToReset: sleepTimerShakeToReset,
    lastDuration: sleepTimerType
  };
  localStorage.setItem('abs-sleep-timer-settings', JSON.stringify(settings));
}

function handleDeviceMotion(event) {
  const acc = event.accelerationIncludingGravity;
  if (!acc) return;
  const threshold = 15;
  const deltaX = Math.abs(acc.x || 0);
  const deltaY = Math.abs(acc.y || 0);
  const deltaZ = Math.abs(acc.z || 0);
  if (deltaX > threshold || deltaY > threshold || deltaZ > threshold) {
    const now = Date.now();
    if (now - lastShakeTime > 2000) {
      lastShakeTime = now;
      resetSleepTimer();
      showNotification('Sleep timer extended via shake!');
    }
  }
}

async function enableShakeDetection() {
  if (typeof DeviceMotionEvent !== 'undefined' && typeof DeviceMotionEvent.requestPermission === 'function') {
    try {
      const permissionState = await DeviceMotionEvent.requestPermission();
      if (permissionState === 'granted') {
        window.addEventListener('devicemotion', handleDeviceMotion);
      }
    } catch (err) {
      console.warn('DeviceMotion permission request rejected:', err);
    }
  } else {
    window.addEventListener('devicemotion', handleDeviceMotion);
  }
}


// Initial initialization of settings from localStorage
(function initSleepTimerSettings() {
  const storedSettings = localStorage.getItem('abs-sleep-timer-settings');
  if (storedSettings) {
    try {
      const parsed = JSON.parse(storedSettings);
      sleepTimerAutoRestart = parsed.autoRestart ?? false;
      sleepTimerShakeToReset = parsed.shakeToReset ?? true;
      sleepTimerType = parsed.lastDuration ?? 'off';
    } catch (e) {
      console.warn('Failed to parse sleep timer settings', e);
    }
  }
})();

async function fetchWaveform(itemId) {
  currentWaveform = null;
  drawWaveform();
  try {
    const token = localStorage.getItem('token');
    const resp = await fetch(resolvePath(`/api/items/${itemId}/waveform`), {
      headers: {
        'Authorization': `Bearer ${token}`
      }
    });
    if (resp.ok) {
      const data = await resp.json();
      if (data.itemId === itemId) {
        currentWaveform = data.peaks;
        drawWaveform();
      }
    }
  } catch (err) {
    console.warn('Failed to fetch waveform:', err);
  }
}


(function initPlayerSettings() {
  const rememberVal = localStorage.getItem('abs-speed-remember-per-book');
  rememberSpeedPerBook = rememberVal !== 'false';
  
  const globalSpeedVal = localStorage.getItem('abs-speed-global');
  if (globalSpeedVal) {
    globalDefaultSpeed = parseFloat(globalSpeedVal) || 1.0;
  }
  
  const boostVal = localStorage.getItem('abs-volume-boost');
  if (boostVal) {
    volumeBoostLevel = parseFloat(boostVal) || 1.0;
  }

  const skipBackVal = localStorage.getItem('abs-player-skip-back');
  if (skipBackVal) {
    playerSkipBackSeconds = parseInt(skipBackVal, 10) || 10;
  }

  const skipForwardVal = localStorage.getItem('abs-player-skip-forward');
  if (skipForwardVal) {
    playerSkipForwardSeconds = parseInt(skipForwardVal, 10) || 10;
  }
})();




// Expose getters/setters/methods to UI and Cast
export function getAudio() { return audio; }
export function getCurrentItem() { return currentItem; }
export function getIsMuted() { return isMuted; }
export function setIsMuted(val) { isMuted = val; }
export function getPreviousVolume() { return previousVolume; }
export function setPreviousVolume(val) { previousVolume = val; }
export function getUserVolume() { return userVolume; }
export function setUserVolume(val) { userVolume = val; }
export function getCurrentWaveform() { return currentWaveform; }
export function getHoverPct() { return hoverPct; }
export function setHoverPct(val) { hoverPct = val; }
export function getHoverX() { return hoverX; }
export function setHoverX(val) { hoverX = val; }
export function getVolumeBoostLevel() { return volumeBoostLevel; }
export function getRememberSpeedPerBook() { return rememberSpeedPerBook; }
export function getGlobalDefaultSpeed() { return globalDefaultSpeed; }
export function setGlobalDefaultSpeed(val) { globalDefaultSpeed = val; }
export function getPlayerSkipBackSeconds() { return playerSkipBackSeconds; }
export function getPlayerSkipForwardSeconds() { return playerSkipForwardSeconds; }
export function getSleepTimerType() { return sleepTimerType; }
export function getSleepTimerTimeRemaining() { return sleepTimerTimeRemaining; }
export function getSleepTimerAutoRestart() { return sleepTimerAutoRestart; }
export function setSleepTimerAutoRestart(val) { sleepTimerAutoRestart = val; }
export function getSleepTimerShakeToReset() { return sleepTimerShakeToReset; }
export function setSleepTimerShakeToReset(val) { sleepTimerShakeToReset = val; }
export function getCurrentPlaylistUri() { return currentPlaylistUri; }
export function isFadingVal() { return isFading; }

export function playNextInQueue() {
  onPlaybackEnded();
}

export async function addBookmark(time, title, note, color) {
  if (!currentItem) return;
  await request('POST', `/api/me/item/${currentItem.id}/bookmark`, {
    time,
    title,
    note,
    color
  });
  const event = new CustomEvent('bookmark-added', { detail: { itemId: currentItem.id } });
  window.dispatchEvent(event);
}

export function saveSettings(settings) {
  rememberSpeedPerBook = settings.rememberSpeedPerBook;
  globalDefaultSpeed = settings.globalDefaultSpeed;
  volumeBoostLevel = settings.volumeBoostLevel;
  playerSkipBackSeconds = settings.playerSkipBackSeconds;
  playerSkipForwardSeconds = settings.playerSkipForwardSeconds;

  localStorage.setItem('abs-speed-remember-per-book', rememberSpeedPerBook.toString());
  localStorage.setItem('abs-speed-global', globalDefaultSpeed.toString());
  localStorage.setItem('abs-volume-boost', volumeBoostLevel.toString());
  localStorage.setItem('abs-player-skip-back', playerSkipBackSeconds.toString());
  localStorage.setItem('abs-player-skip-forward', playerSkipForwardSeconds.toString());

  if (audio) {
    let speedToUse = globalDefaultSpeed;
    if (rememberSpeedPerBook && currentItem) {
      const storedSpeed = localStorage.getItem(`abs-speed-book-${currentItem.id}`);
      if (storedSpeed) {
        speedToUse = parseFloat(storedSpeed);
      }
    }
    audio.playbackRate = speedToUse;
    const speedSelect = document.getElementById('player-speed');
    if (speedSelect) {
      speedSelect.value = speedToUse.toString();
    }
  }

  if (gainNode) {
    gainNode.gain.value = volumeBoostLevel;
  } else if (volumeBoostLevel > 1.0 && audio) {
    initVolumeBoost();
  }
}

// Initialize player UI controller connection
setPlayerController({
  getAudio,
  getCurrentItem,
  getIsMuted,
  setIsMuted,
  getPreviousVolume,
  setPreviousVolume,
  getUserVolume,
  setUserVolume,
  getCurrentWaveform,
  getHoverPct,
  setHoverPct,
  getHoverX,
  setHoverX,
  getVolumeBoostLevel,
  getRememberSpeedPerBook,
  getGlobalDefaultSpeed,
  getPlayerSkipBackSeconds,
  getPlayerSkipForwardSeconds,
  getSleepTimerType,
  getSleepTimerTimeRemaining,
  getSleepTimerAutoRestart,
  getSleepTimerShakeToReset,
  startSleepTimer,
  stopSleepTimer,
  resetSleepTimer,
  saveSleepTimerSettings,
  getCurrentPlaylistUri,
  playItem,
  pauseItem,
  seekTo,
  destroyPlayer,
  playNextInQueue,
  isFading: () => isFading,
  saveSettings,
  addBookmark
});

registerNotificationCallback(showNotification);

// Initialize Cast callbacks
setupCastCallbacks({
  onConnected: () => {
    const localTime = audio ? audio.currentTime : 0;
    if (audio) {
      audio.pause();
      audio.src = '';
    }
    if (currentItem && currentPlaylistUri) {
      castPlayItem(currentItem, localTime, currentPlaylistUri);
    }
  },
  onDisconnected: (remoteTime) => {
    if (currentItem) {
      playItem(currentItem, remoteTime);
    }
  },
  onStateChanged: (isPlaying, isFinished) => {
    updatePlayPauseButton(isPlaying);
    if (isFinished) {
      onPlaybackEnded();
    }
  },
  onTimeChanged: () => {
    updateTimelineUI();
  },
  onDurationChanged: () => {
    updateTimelineUI();
  }
});
