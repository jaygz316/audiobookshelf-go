// js/player.js

import { request, resolvePath } from './api.js';

let audio = null;
let hls = null;
let currentItem = null;
let progressInterval = null;
let isMuted = false;
let previousVolume = 1.0;

// Initialize audio and elements
function initAudio() {
  if (audio) return;
  audio = new Audio();
  
  // Set up audio events for timeline and status updates
  audio.addEventListener('timeupdate', updateTimelineUI);
  audio.addEventListener('durationchange', updateTimelineUI);
  audio.addEventListener('ended', onPlaybackEnded);
  audio.addEventListener('play', () => {
    updatePlayPauseButton(true);
  });
  audio.addEventListener('pause', () => {
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
    
    // Stop any existing playback and progress reporting
    stopProgressReporting();
    if (hls) {
      hls.destroy();
      hls = null;
    }
    
    // Set metadata on UI
    updateMetadataUI(item);
    
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
  if (!currentItem || !audio) return;
  
  const payload = {
    currentTime: audio.currentTime || 0,
    duration: audio.duration || 0,
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
      if (!audio) return;
      if (audio.paused) {
        audio.play().catch(err => console.error('Play failed:', err));
      } else {
        audio.pause();
      }
    };
  }
  
  if (seekBackBtn) {
    seekBackBtn.onclick = () => {
      if (!audio) return;
      audio.currentTime = Math.max(audio.currentTime - 10, 0);
    };
  }
  
  if (seekForwardBtn) {
    seekForwardBtn.onclick = () => {
      if (!audio) return;
      audio.currentTime = Math.min(audio.currentTime + 10, audio.duration || 0);
    };
  }
  
  if (timeline) {
    timeline.oninput = () => {
      if (!audio || !audio.duration) return;
      const pct = parseFloat(timeline.value) / 100;
      audio.currentTime = pct * audio.duration;
    };
  }
  
  if (volumeBtn) {
    volumeBtn.onclick = () => {
      if (!audio) return;
      isMuted = !isMuted;
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
    };
  }
  
  if (volumeSlider) {
    volumeSlider.oninput = () => {
      if (!audio) return;
      const val = parseFloat(volumeSlider.value) / 100;
      audio.volume = val;
      isMuted = val === 0;
      updateVolumeIcon(val);
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
  if (!audio || !audio.duration) return;
  
  const elapsed = audio.currentTime || 0;
  const duration = audio.duration;
  
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
    timeline.value = Math.round((elapsed / duration) * 100);
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
  currentItem = null;
  
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
