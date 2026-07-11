// js/player.js

import { request, resolvePath } from './api.js';

let audio = null;
let hls = null;
let currentItem = null;
let currentPlaylistUri = null;
let progressInterval = null;
let isMuted = false;
let previousVolume = 1.0;

// Google Cast fields
let castContext = null;
let remotePlayer = null;
let remotePlayerController = null;

// Initialize Cast API
function initializeCastApi() {
  if (!window.cast || !window.cast.framework) {
    console.warn('Cast framework not available yet');
    return;
  }
  
  castContext = window.cast.framework.CastContext.getInstance();
  castContext.setOptions({
    receiverApplicationId: chrome.cast.media.DEFAULT_MEDIA_RECEIVER_APP_ID,
    autoJoinPolicy: chrome.cast.AutoJoinPolicy.ORIGINAL_SESSION_RESTRICTION
  });

  remotePlayer = new window.cast.framework.RemotePlayer();
  remotePlayerController = new window.cast.framework.RemotePlayerController(remotePlayer);

  // Listen for connection status changes
  remotePlayerController.addEventListener(
    window.cast.framework.RemotePlayerEventType.IS_CONNECTED_CHANGED,
    onCastConnectionChanged
  );

  // Listen for media status changes (play/pause state, time, etc.)
  remotePlayerController.addEventListener(
    window.cast.framework.RemotePlayerEventType.PLAYER_STATE_CHANGED,
    onCastStateChanged
  );

  remotePlayerController.addEventListener(
    window.cast.framework.RemotePlayerEventType.CURRENT_TIME_CHANGED,
    onCastTimeChanged
  );

  remotePlayerController.addEventListener(
    window.cast.framework.RemotePlayerEventType.DURATION_CHANGED,
    onCastDurationChanged
  );
}

// Attach callback to window
window.__onGCastApiAvailable = function(isAvailable) {
  if (isAvailable) {
    initializeCastApi();
  }
};

// Check if already available (safeguard)
if (window.cast && window.cast.framework) {
  initializeCastApi();
}

function onCastConnectionChanged() {
  if (remotePlayer && remotePlayer.isConnected) {
    console.log('Chromecast connected');
    const localTime = audio ? audio.currentTime : 0;
    if (audio) {
      audio.pause();
      audio.src = '';
    }
    if (currentItem && currentPlaylistUri) {
      castPlayItem(currentItem, localTime, currentPlaylistUri);
    }
  } else {
    console.log('Chromecast disconnected');
    if (currentItem && remotePlayer) {
      const remoteTime = remotePlayer.currentTime || 0;
      playItem(currentItem, remoteTime);
    }
  }
}

function onCastStateChanged() {
  if (!remotePlayer) return;
  const state = remotePlayer.playerState;
  const isPlaying = state === chrome.cast.media.PlayerState.PLAYING;
  updatePlayPauseButton(isPlaying);

  if (state === chrome.cast.media.PlayerState.IDLE && remotePlayer.idleReason === chrome.cast.media.IdleReason.FINISHED) {
    onPlaybackEnded();
  }
}

function onCastTimeChanged() {
  updateTimelineUI();
}

function onCastDurationChanged() {
  updateTimelineUI();
}

function castPlayItem(item, startTime = 0, clientPlaylistUri) {
  if (!castContext) return;
  const session = castContext.getCurrentSession();
  if (!session) return;

  // Build the absolute streaming URL
  let streamUrl = window.location.origin + resolvePath(clientPlaylistUri);

  const mediaInfo = new chrome.cast.media.MediaInfo(streamUrl, 'application/vnd.apple.mpegurl');
  mediaInfo.metadata = new chrome.cast.media.GenericMediaMetadata();
  mediaInfo.metadata.metadataType = chrome.cast.media.MetadataType.MUSIC_TRACK;
  
  let title = '';
  let author = '';
  if (item.mediaType === 'book') {
    const metadata = item.media?.metadata || {};
    title = metadata.title || item.title || 'Untitled';
    author = metadata.authorName || 'Unknown';
  } else if (item.mediaType === 'podcast') {
    const metadata = item.media?.metadata || {};
    title = metadata.title || item.title || 'Untitled';
    author = metadata.author || 'Unknown';
  } else {
    title = item.title || 'Untitled';
    author = 'Unknown';
  }

  mediaInfo.metadata.title = title;
  mediaInfo.metadata.artist = author;
  
  const token = localStorage.getItem('token');
  const ts = item.updatedAt || item.addedAt || Date.now();
  const coverUrl = window.location.origin + resolvePath(`/api/items/${item.id}/cover?token=${token}&ts=${ts}`);
  mediaInfo.metadata.images = [{ url: coverUrl }];

  const requestObj = new chrome.cast.media.LoadRequest(mediaInfo);
  requestObj.currentTime = startTime;

  session.loadMedia(requestObj).then(
    function() {
      console.log('Chromecast: loadMedia success');
      updatePlayPauseButton(true);
    },
    function(err) {
      console.error('Chromecast: loadMedia error', err);
    }
  );
}

// Initialize audio and elements
function initAudio() {
  if (audio) return;
  audio = new Audio();
  
  // Set up audio events for timeline and status updates
  audio.addEventListener('timeupdate', updateTimelineUI);
  audio.addEventListener('durationchange', updateTimelineUI);
  audio.addEventListener('ended', onPlaybackEnded);
  audio.addEventListener('play', () => {
    if (remotePlayer && remotePlayer.isConnected) return;
    updatePlayPauseButton(true);
  });
  audio.addEventListener('pause', () => {
    if (remotePlayer && remotePlayer.isConnected) return;
    updatePlayPauseButton(false);
  });
  
  setupUIEventListeners();
}

export async function playItem(item, startTime = 0) {
  initAudio();
  
  try {
    // 1. Create playback session
    const playResponse = await request('POST', `/api/items/${item.id}/play`, { startTime });
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
    
    currentItem = item;
    currentPlaylistUri = clientPlaylistUri;
    
    // Stop any existing playback and progress reporting
    stopProgressReporting();
    if (hls) {
      hls.destroy();
      hls = null;
    }
    
    // Set metadata on UI
    updateMetadataUI(item);
    
    if (remotePlayer && remotePlayer.isConnected) {
      if (audio) {
        audio.pause();
        audio.src = '';
      }
      castPlayItem(item, startTime, clientPlaylistUri);
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
        audio.volume = parseFloat(volumeSlider.value) / 100;
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
  if (remotePlayer && remotePlayer.isConnected) {
    currentTime = remotePlayer.currentTime || 0;
    duration = remotePlayer.duration || 0;
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
    await request('PATCH', `/api/me/progress/${currentItem.id}`, payload);
  } catch (err) {
    console.warn('Failed to save playback progress:', err);
  }
}

// Setup Event Listeners for Player UI Buttons/Sliders
function setupUIEventListeners() {
  const playPauseBtn = document.getElementById('player-play-pause');
  const seekBackBtn = document.getElementById('player-seek-back');
  const seekForwardBtn = document.getElementById('player-seek-forward');
  const timeline = document.getElementById('player-timeline');
  const volumeBtn = document.getElementById('player-volume-btn');
  const volumeSlider = document.getElementById('player-volume-slider');
  const speedSelect = document.getElementById('player-speed');
  const closeBtn = document.getElementById('player-close');
  
  if (playPauseBtn) {
    playPauseBtn.onclick = () => {
      if (remotePlayer && remotePlayer.isConnected) {
        remotePlayerController.playOrPause();
      } else {
        if (!audio) return;
        if (audio.paused) {
          audio.play().catch(err => console.error('Play failed:', err));
        } else {
          audio.pause();
        }
      }
    };
  }
  
  if (seekBackBtn) {
    seekBackBtn.onclick = () => {
      if (remotePlayer && remotePlayer.isConnected) {
        const newTime = Math.max(remotePlayer.currentTime - 10, 0);
        remotePlayer.currentTime = newTime;
        remotePlayerController.seek();
      } else {
        if (!audio) return;
        audio.currentTime = Math.max(audio.currentTime - 10, 0);
      }
    };
  }
  
  if (seekForwardBtn) {
    seekForwardBtn.onclick = () => {
      if (remotePlayer && remotePlayer.isConnected) {
        const newTime = Math.min(remotePlayer.currentTime + 10, remotePlayer.duration || 0);
        remotePlayer.currentTime = newTime;
        remotePlayerController.seek();
      } else {
        if (!audio) return;
        audio.currentTime = Math.min(audio.currentTime + 10, audio.duration || 0);
      }
    };
  }
  
  if (timeline) {
    timeline.oninput = () => {
      if (remotePlayer && remotePlayer.isConnected) {
        if (!remotePlayer.duration) return;
        const pct = parseFloat(timeline.value) / 100;
        remotePlayer.currentTime = pct * remotePlayer.duration;
        remotePlayerController.seek();
      } else {
        if (!audio || !audio.duration) return;
        const pct = parseFloat(timeline.value) / 100;
        audio.currentTime = pct * audio.duration;
      }
    };
  }
  
  if (volumeBtn) {
    volumeBtn.onclick = () => {
      isMuted = !isMuted;
      if (remotePlayer && remotePlayer.isConnected) {
        if (isMuted) {
          previousVolume = remotePlayer.volumeLevel;
          remotePlayer.volumeLevel = 0;
          remotePlayerController.setVolumeLevel();
          updateVolumeIcon(0);
          if (volumeSlider) volumeSlider.value = 0;
        } else {
          remotePlayer.volumeLevel = previousVolume;
          remotePlayerController.setVolumeLevel();
          updateVolumeIcon(previousVolume);
          if (volumeSlider) volumeSlider.value = Math.round(previousVolume * 100);
        }
      } else {
        if (!audio) return;
        if (isMuted) {
          previousVolume = audio.volume;
          audio.volume = 0;
          updateVolumeIcon(0);
          if (volumeSlider) volumeSlider.value = 0;
        } else {
          audio.volume = previousVolume;
          updateVolumeIcon(previousVolume);
          if (volumeSlider) volumeSlider.value = Math.round(previousVolume * 100);
        }
      }
    };
  }
  
  if (volumeSlider) {
    volumeSlider.oninput = () => {
      const val = parseFloat(volumeSlider.value) / 100;
      isMuted = val === 0;
      updateVolumeIcon(val);
      if (remotePlayer && remotePlayer.isConnected) {
        remotePlayer.volumeLevel = val;
        remotePlayerController.setVolumeLevel();
      } else {
        if (!audio) return;
        audio.volume = val;
      }
    };
  }
  
  if (speedSelect) {
    speedSelect.onchange = () => {
      if (!audio) return;
      audio.playbackRate = parseFloat(speedSelect.value) || 1.0;
    };
  }
  
  if (closeBtn) {
    closeBtn.onclick = () => {
      destroyPlayer();
    };
  }

  const bookmarkBtn = document.getElementById('player-bookmark-btn');
  if (bookmarkBtn) {
    bookmarkBtn.onclick = () => {
      triggerAddBookmarkModal();
    };
  }
}

// Format duration/time helper
function formatTime(secs) {
  if (isNaN(secs) || secs === Infinity) return '0:00';
  const hours = Math.floor(secs / 3600);
  const minutes = Math.floor((secs % 3600) / 60);
  const seconds = Math.floor(secs % 60);
  
  const formattedSeconds = seconds < 10 ? `0${seconds}` : seconds;
  if (hours > 0) {
    const formattedMinutes = minutes < 10 ? `0${minutes}` : minutes;
    return `${hours}:${formattedMinutes}:${formattedSeconds}`;
  }
  return `${minutes}:${formattedSeconds}`;
}

// Update UI Helpers
function updateTimelineUI() {
  let elapsed = 0;
  let duration = 0;
  
  if (remotePlayer && remotePlayer.isConnected) {
    elapsed = remotePlayer.currentTime || 0;
    duration = remotePlayer.duration || 0;
  } else {
    if (!audio || !audio.duration) return;
    elapsed = audio.currentTime || 0;
    duration = audio.duration;
  }
  
  const elapsedEl = document.getElementById('player-time-elapsed');
  const remainingEl = document.getElementById('player-time-remaining');
  const timeline = document.getElementById('player-timeline');
  
  if (elapsedEl) {
    elapsedEl.textContent = formatTime(elapsed);
  }
  if (remainingEl) {
    remainingEl.textContent = formatTime(duration - elapsed);
  }
  if (timeline) {
    timeline.value = duration > 0 ? Math.round((elapsed / duration) * 100) : 0;
  }
}

function updatePlayPauseButton(isPlaying) {
  const icon = document.getElementById('player-play-pause-icon');
  if (icon) {
    icon.textContent = isPlaying ? 'pause' : 'play_arrow';
  }
}

function updateVolumeIcon(vol) {
  const icon = document.getElementById('player-volume-icon');
  if (!icon) return;
  
  if (vol === 0) {
    icon.textContent = 'volume_mute';
  } else if (vol < 0.5) {
    icon.textContent = 'volume_down';
  } else {
    icon.textContent = 'volume_up';
  }
}

function updateMetadataUI(item) {
  const titleEl = document.getElementById('player-title');
  const authorEl = document.getElementById('player-author');
  const coverEl = document.getElementById('player-cover');
  
  let title = '';
  let author = '';
  if (item.mediaType === 'book') {
    const metadata = item.media?.metadata || {};
    title = metadata.title || item.title || 'Untitled';
    author = metadata.authorName || 'Unknown';
  } else if (item.mediaType === 'podcast') {
    const metadata = item.media?.metadata || {};
    title = metadata.title || item.title || 'Untitled';
    author = metadata.author || 'Unknown';
  } else {
    title = item.title || 'Untitled';
    author = 'Unknown';
  }
  
  if (titleEl) titleEl.textContent = title;
  if (authorEl) authorEl.textContent = author;
  
  if (coverEl) {
    const token = localStorage.getItem('token');
    const ts = item.updatedAt || item.addedAt || Date.now();
    coverEl.src = resolvePath(`/api/items/${item.id}/cover?token=${token}&ts=${ts}`);
  }
}

async function onPlaybackEnded() {
  await reportProgress(true);
  destroyPlayer();
}

function destroyPlayer() {
  stopProgressReporting();
  if (audio) {
    audio.pause();
    audio.src = '';
  }
  if (hls) {
    hls.destroy();
    hls = null;
  }
  if (remotePlayer && remotePlayer.isConnected && castContext) {
    const session = castContext.getCurrentSession();
    if (session) {
      session.endSession(true);
    }
  }
  currentItem = null;
  currentPlaylistUri = null;
  
  const playerBar = document.getElementById('player-bar');
  if (playerBar) {
    playerBar.classList.add('hidden');
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

function triggerAddBookmarkModal() {
  if (!currentItem) return;
  let time = 0;
  if (remotePlayer && remotePlayer.isConnected) {
    time = remotePlayer.currentTime || 0;
  } else {
    if (!audio) return;
    time = audio.currentTime || 0;
  }
  
  // Format current position to HH:MM:SS format as placeholder
  let hrs = Math.floor(time / 3600);
  let mins = Math.floor((time % 3600) / 60);
  let secs = Math.floor(time % 60);
  let timeStr = "";
  if (hrs > 0) {
    timeStr += `${hrs}:${mins < 10 ? '0' : ''}${mins}:${secs < 10 ? '0' : ''}${secs}`;
  } else {
    timeStr += `${mins}:${secs < 10 ? '0' : ''}${secs}`;
  }

  const defaultTitle = `Bookmark at ${timeStr}`;

  // Create Modal
  const modal = document.createElement('div');
  modal.className = 'fixed inset-0 bg-black-900/80 backdrop-blur-sm z-50 flex items-center justify-center p-4 select-none';
  modal.innerHTML = `
    <div class="bg-primary border border-black-400 rounded-lg max-w-md w-full p-6 shadow-2xl space-y-4">
      <div class="flex items-center justify-between border-b border-black-500 pb-3">
        <h3 class="text-sm font-bold text-white uppercase tracking-wider flex items-center space-x-1.5">
          <span class="material-symbols text-base text-accent">bookmark</span>
          <span>Add Bookmark</span>
        </h3>
        <button id="close-bookmark-modal" class="text-black-100 hover:text-white transition-colors">
          <span class="material-symbols text-lg">close</span>
        </button>
      </div>

      <div class="space-y-3 text-left">
        <div>
          <label class="text-[0.65rem] uppercase font-bold text-black-100 tracking-wider">Bookmark Time</label>
          <p class="text-white font-semibold text-sm">${timeStr}</p>
        </div>
        <div>
          <label for="bookmark-title-input" class="text-[0.65rem] uppercase font-bold text-black-100 tracking-wider block mb-1">Bookmark Title</label>
          <input type="text" id="bookmark-title-input" required class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent text-xs" placeholder="e.g. Chapter 2 Start">
        </div>
      </div>

      <div class="flex items-center justify-end space-x-3 pt-3 border-t border-black-500">
        <button id="cancel-bookmark-btn" class="bg-transparent hover:bg-black-500 text-white px-4 py-2 rounded text-xs transition-colors">
          Cancel
        </button>
        <button id="save-bookmark-btn" class="bg-accent text-primary font-bold px-4 py-2 rounded text-xs hover:opacity-90 transition-opacity">
          Save Bookmark
        </button>
      </div>
    </div>
  `;

  document.body.appendChild(modal);

  const titleInput = document.getElementById('bookmark-title-input');
  titleInput.value = defaultTitle;
  titleInput.focus();
  titleInput.select();

  const closeModal = () => modal.remove();
  document.getElementById('close-bookmark-modal').onclick = closeModal;
  document.getElementById('cancel-bookmark-btn').onclick = closeModal;

  document.getElementById('save-bookmark-btn').onclick = async () => {
    const titleVal = titleInput.value.trim();
    if (!titleVal) {
      alert("Title is required");
      return;
    }

    try {
      await request('POST', `/api/me/item/${currentItem.id}/bookmark`, {
        time: time,
        title: titleVal
      });
      closeModal();
      
      // Notify details page if it is open
      const event = new CustomEvent('bookmark-added', { detail: { itemId: currentItem.id } });
      window.dispatchEvent(event);

    } catch (err) {
      console.error('Failed to create bookmark:', err);
      alert('Failed to save bookmark: ' + (err.message || 'Unknown error'));
    }
  };
}

export function getCurrentPlayingItem() {
  return currentItem;
}

export function getCurrentPlaybackTime() {
  if (remotePlayer && remotePlayer.isConnected) {
    return remotePlayer.currentTime || 0;
  }
  return audio ? audio.currentTime : 0;
}
