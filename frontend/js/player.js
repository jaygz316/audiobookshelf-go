// js/player.js

import { request, resolvePath } from './api.js';

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
  audio.crossOrigin = "anonymous";
  
  // Set up audio events for timeline and status updates
  audio.addEventListener('timeupdate', updateTimelineUI);
  audio.addEventListener('durationchange', updateTimelineUI);
  audio.addEventListener('ended', onPlaybackEnded);
  audio.addEventListener('play', () => {
    if (remotePlayer && remotePlayer.isConnected) return;
    updatePlayPauseButton(true);
    
    // Auto-restart sleep timer if enabled
    if (sleepTimerAutoRestart && sleepTimerType !== 'off' && !isSleepTimerActive) {
      startSleepTimer(sleepTimerType);
    }
  });
  audio.addEventListener('pause', () => {
    if (remotePlayer && remotePlayer.isConnected) return;
    updatePlayPauseButton(false);
    
    // Clear/stop sleep timer on pause, but preserve sleepTimerType for auto-restart
    if (isSleepTimerActive) {
      stopSleepTimer(false); // false means don't clear the saved type
    }
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
  playbackQueue = items.slice(startIndex + 1);
  const itemToPlay = items[startIndex];
  updateQueueUI();
  await playItem(itemToPlay);
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
    
    if (remotePlayer && remotePlayer.isConnected) {
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
    const progressUrl = currentItem.episodeId 
      ? `/api/me/progress/${currentItem.id}/${currentItem.episodeId}` 
      : `/api/me/progress/${currentItem.id}`;
    await request('PATCH', progressUrl, payload);
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
  
  const prevChapterBtn = document.getElementById('player-prev-chapter');
  if (prevChapterBtn) {
    prevChapterBtn.onclick = () => {
      const currentSecs = remotePlayer && remotePlayer.isConnected ? remotePlayer.currentTime : (audio ? audio.currentTime : 0);
      const chapters = currentItem?.media?.chapters || [];
      const activeChapter = chapters.find(c => currentSecs >= c.start && currentSecs < c.end);
      const activeChapterIndex = chapters.indexOf(activeChapter);
      
      if (activeChapterIndex === -1 || activeChapterIndex === 0) {
        seekTo(0);
      } else {
        const timeInCurrentChapter = currentSecs - activeChapter.start;
        if (timeInCurrentChapter <= 3 && chapters[activeChapterIndex - 1]) {
          seekTo(chapters[activeChapterIndex - 1].start);
        } else {
          seekTo(activeChapter.start);
        }
      }
    };
  }

  if (seekBackBtn) {
    seekBackBtn.onclick = () => {
      if (remotePlayer && remotePlayer.isConnected) {
        const newTime = Math.max(remotePlayer.currentTime - playerSkipBackSeconds, 0);
        remotePlayer.currentTime = newTime;
        remotePlayerController.seek();
      } else {
        if (!audio) return;
        audio.currentTime = Math.max(audio.currentTime - playerSkipBackSeconds, 0);
      }
    };
  }
  
  if (seekForwardBtn) {
    seekForwardBtn.onclick = () => {
      if (remotePlayer && remotePlayer.isConnected) {
        const newTime = Math.min(remotePlayer.currentTime + playerSkipForwardSeconds, remotePlayer.duration || 0);
        remotePlayer.currentTime = newTime;
        remotePlayerController.seek();
      } else {
        if (!audio) return;
        audio.currentTime = Math.min(audio.currentTime + playerSkipForwardSeconds, audio.duration || 0);
      }
    };
  }

  const nextBtn = document.getElementById('player-next');
  if (nextBtn) {
    nextBtn.onclick = () => {
      const chapters = currentItem?.media?.chapters || [];
      const currentSecs = remotePlayer && remotePlayer.isConnected ? remotePlayer.currentTime : (audio ? audio.currentTime : 0);
      const activeChapter = chapters.find(c => currentSecs >= c.start && currentSecs < c.end);
      const activeChapterIndex = chapters.indexOf(activeChapter);
      
      if (activeChapterIndex !== -1 && activeChapterIndex < chapters.length - 1) {
        const nextChapter = chapters[activeChapterIndex + 1];
        seekTo(nextChapter.start);
      } else if (playbackQueue.length > 0) {
        playNextInQueue();
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
      drawWaveform();
    };

    // Waveform interactive hover seeking and tooltip preview
    let tooltip = document.getElementById('player-waveform-tooltip');
    if (!tooltip && timeline.parentElement) {
      tooltip = document.createElement('div');
      tooltip.id = 'player-waveform-tooltip';
      tooltip.className = 'absolute bg-black-700 text-white text-[10px] px-1.5 py-0.5 rounded border border-black-300 pointer-events-none hidden z-30 transition-opacity duration-150 shadow-md font-bold';
      tooltip.style.bottom = '100%';
      tooltip.style.marginBottom = '6px';
      tooltip.style.transform = 'translateX(-50%)';
      timeline.parentElement.appendChild(tooltip);
    }

    timeline.addEventListener('mouseenter', () => {
      if (tooltip) tooltip.classList.remove('hidden');
    });

    timeline.addEventListener('mousemove', (e) => {
      const rect = timeline.getBoundingClientRect();
      const x = e.clientX - rect.left;
      const pct = Math.max(0, Math.min(1, x / rect.width));
      
      hoverPct = pct;
      hoverX = x;

      const duration = (remotePlayer && remotePlayer.isConnected) ? (remotePlayer.duration || 0) : (audio ? audio.duration : 0);
      if (duration > 0) {
        const hoverTime = pct * duration;
        if (tooltip) {
          const chapters = currentItem?.media?.chapters || [];
          const activeChap = chapters.find(c => hoverTime >= c.start && hoverTime < c.end);
          if (activeChap) {
            tooltip.textContent = `${activeChap.title || 'Untitled'} (${formatTime(hoverTime)})`;
          } else {
            tooltip.textContent = formatTime(hoverTime);
          }
          tooltip.style.left = `${pct * 100}%`;
        }
      }
      drawWaveform();
    });

    timeline.addEventListener('mouseleave', () => {
      hoverPct = null;
      hoverX = null;
      if (tooltip) tooltip.classList.add('hidden');
      drawWaveform();
    });
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
          previousVolume = userVolume;
          userVolume = 0;
          if (!isFading) {
            audio.volume = 0;
          }
          updateVolumeIcon(0);
          if (volumeSlider) volumeSlider.value = 0;
        } else {
          userVolume = previousVolume;
          if (!isFading) {
            audio.volume = userVolume;
          }
          updateVolumeIcon(userVolume);
          if (volumeSlider) volumeSlider.value = Math.round(userVolume * 100);
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
        userVolume = val;
        if (!isFading) {
          audio.volume = val;
        }
      }
    };
  }
  
  if (speedSelect) {
    speedSelect.onchange = () => {
      if (!audio) return;
      const speedVal = parseFloat(speedSelect.value) || 1.0;
      audio.playbackRate = speedVal;
      
      // Persist chosen speed
      if (rememberSpeedPerBook && currentItem) {
        localStorage.setItem(`abs-speed-book-${currentItem.id}`, speedVal.toString());
      } else {
        globalDefaultSpeed = speedVal;
        localStorage.setItem('abs-speed-global', speedVal.toString());
      }
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

  const sleepBtn = document.getElementById('player-sleep-btn');
  if (sleepBtn) {
    sleepBtn.onclick = () => {
      triggerSleepTimerModal();
    };
  }

  const settingsBtn = document.getElementById('player-settings-btn');
  if (settingsBtn) {
    settingsBtn.onclick = () => {
      triggerPlayerSettingsModal();
    };
  }

  const chaptersBtn = document.getElementById('player-chapters-btn');
  if (chaptersBtn) {
    chaptersBtn.onclick = () => {
      triggerChaptersModal();
    };
  }

  const queueBtn = document.getElementById('player-queue-btn');
  if (queueBtn) {
    queueBtn.onclick = () => {
      triggerQueueModal();
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
  drawWaveform();

  const chapterInfo = document.getElementById('player-chapter-info');
  if (chapterInfo) {
    const chapters = currentItem?.media?.chapters || [];
    const activeChapter = chapters.find(c => elapsed >= c.start && elapsed < c.end);
    if (activeChapter) {
      const activeChapterIndex = chapters.indexOf(activeChapter);
      chapterInfo.textContent = `Chapter ${activeChapterIndex + 1} of ${chapters.length}: ${activeChapter.title || 'Untitled'}`;
      chapterInfo.classList.remove('hidden');
    } else {
      chapterInfo.classList.add('hidden');
    }
  }

  updatePlaybackControlsUI();
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

  const chaptersBtn = document.getElementById('player-chapters-btn');
  const chapters = item.media?.chapters || [];
  if (chaptersBtn) {
    if (chapters.length > 0) {
      chaptersBtn.classList.remove('hidden');
    } else {
      chaptersBtn.classList.add('hidden');
    }
  }

  updatePlaybackControlsUI();
  updateSkipButtonsUI();
}

async function onPlaybackEnded() {
  await reportProgress(true);
  if (playbackQueue && playbackQueue.length > 0) {
    const nextItem = playbackQueue.shift();
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

function updateSkipButtonsUI() {
  const seekBackBtn = document.getElementById('player-seek-back');
  const seekForwardBtn = document.getElementById('player-seek-forward');
  if (seekBackBtn) {
    seekBackBtn.title = `Seek Back ${playerSkipBackSeconds}s`;
    const icon = seekBackBtn.querySelector('.material-symbols');
    if (icon) {
      if ([5, 10, 15, 30, 45, 60].includes(playerSkipBackSeconds)) {
        icon.textContent = `replay_${playerSkipBackSeconds}`;
      } else {
        icon.textContent = 'replay';
      }
    }
  }
  if (seekForwardBtn) {
    seekForwardBtn.title = `Seek Forward ${playerSkipForwardSeconds}s`;
    const icon = seekForwardBtn.querySelector('.material-symbols');
    if (icon) {
      if ([5, 10, 15, 30, 45, 60].includes(playerSkipForwardSeconds)) {
        icon.textContent = `forward_${playerSkipForwardSeconds}`;
      } else {
        icon.textContent = 'forward_media';
      }
    }
  }
}

function updatePlaybackControlsUI() {
  const prevChapterBtn = document.getElementById('player-prev-chapter');
  const nextBtn = document.getElementById('player-next');
  if (!prevChapterBtn || !nextBtn) return;

  const chapters = currentItem?.media?.chapters || [];
  
  if (chapters.length > 0) {
    prevChapterBtn.classList.remove('hidden');
    nextBtn.classList.remove('hidden');
  } else if (playbackQueue.length > 0) {
    prevChapterBtn.classList.add('hidden');
    nextBtn.classList.remove('hidden');
  } else {
    prevChapterBtn.classList.add('hidden');
    nextBtn.classList.add('hidden');
  }
}

function seekTo(seconds) {
  if (remotePlayer && remotePlayer.isConnected) {
    remotePlayer.currentTime = seconds;
    remotePlayerController.seek();
  } else {
    if (!audio) return;
    audio.currentTime = seconds;
  }
  updateTimelineUI();
}

function triggerChaptersModal() {
  if (!currentItem || !currentItem.media || !currentItem.media.chapters) return;
  const chapters = currentItem.media.chapters;
  if (chapters.length === 0) return;

  const currentSecs = remotePlayer && remotePlayer.isConnected ? remotePlayer.currentTime : (audio ? audio.currentTime : 0);
  const activeChapter = chapters.find(c => currentSecs >= c.start && currentSecs < c.end);

  const dialog = document.createElement('dialog');
  dialog.id = 'player-chapters-dialog';
  dialog.setAttribute('closedby', 'any');
  dialog.className = 'bg-primary border border-black-400 rounded-lg max-w-md w-full p-6 shadow-2xl space-y-4 focus:outline-none select-none text-white backdrop:bg-black-900/80 backdrop:backdrop-blur-sm open:flex open:flex-col open:items-stretch max-h-[80vh]';

  let chaptersListHtml = '';
  chapters.forEach((chapter, index) => {
    const isActive = activeChapter === chapter;
    const duration = chapter.end - chapter.start;
    chaptersListHtml += `
      <div class="chapter-item flex items-center justify-between p-2.5 rounded cursor-pointer transition-colors ${isActive ? 'bg-black-400 border border-accent/30 text-accent font-semibold' : 'hover:bg-black-500 text-black-50 hover:text-white'}" data-start="${chapter.start}">
        <div class="flex items-center space-x-2.5 overflow-hidden">
          <span class="text-[10px] text-black-100 min-w-[20px] text-right">${index + 1}</span>
          <span class="text-xs truncate">${chapter.title || `Chapter ${index + 1}`}</span>
        </div>
        <div class="flex items-center space-x-3 flex-shrink-0 text-[10px] text-black-100">
          <span>${formatTime(chapter.start)}</span>
          <span class="w-12 text-right">(${formatTime(duration)})</span>
        </div>
      </div>
    `;
  });

  dialog.innerHTML = `
    <div class="flex items-center justify-between border-b border-black-500 pb-3 flex-shrink-0">
      <h3 class="text-sm font-bold text-white uppercase tracking-wider flex items-center space-x-1.5">
        <span class="material-symbols text-base text-accent">format_list_bulleted</span>
        <span>Chapters</span>
      </h3>
      <button id="close-chapters-modal" class="text-black-100 hover:text-white transition-colors">
        <span class="material-symbols text-lg">close</span>
      </button>
    </div>

    <div class="overflow-y-auto flex-grow pr-1 space-y-1 custom-scrollbar max-h-[50vh]">
      ${chaptersListHtml}
    </div>
  `;

  document.body.appendChild(dialog);

  const closeModal = () => {
    dialog.close();
    dialog.remove();
  };

  if (!('closedBy' in HTMLDialogElement.prototype)) {
    dialog.addEventListener('click', (event) => {
      if (event.target !== dialog) return;
      const rect = dialog.getBoundingClientRect();
      const isDialogContent = (
        rect.top <= event.clientY &&
        event.clientY <= rect.top + rect.height &&
        rect.left <= event.clientX &&
        event.clientX <= rect.left + rect.width
      );
      if (isDialogContent) return;
      closeModal();
    });
  }

  dialog.querySelector('#close-chapters-modal').onclick = closeModal;

  dialog.querySelectorAll('.chapter-item').forEach(item => {
    item.onclick = () => {
      const startTime = parseFloat(item.dataset.start);
      seekTo(startTime);
      closeModal();
    };
  });

  dialog.showModal();

  // Scroll active chapter into view
  const activeEl = dialog.querySelector('.bg-black-400');
  if (activeEl) {
    activeEl.scrollIntoView({ block: 'nearest', behavior: 'auto' });
  }
}

function destroyPlayer() {
  stopProgressReporting();
  stopSleepTimer(true);
  window.removeEventListener('devicemotion', handleDeviceMotion);
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
        <div>
          <label for="bookmark-note-input" class="text-[0.65rem] uppercase font-bold text-black-100 tracking-wider block mb-1">Notes</label>
          <textarea id="bookmark-note-input" rows="2" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent text-xs placeholder-black-200" placeholder="Optional notes..."></textarea>
        </div>
        <div>
          <label class="text-[0.65rem] uppercase font-bold text-black-100 tracking-wider block mb-1.5">Color Tag</label>
          <div class="flex items-center space-x-2" id="bookmark-color-options">
            <button type="button" class="w-6 h-6 rounded-full border-2 border-accent transition-all color-option-btn" data-color="#e5a93c" style="background-color: #e5a93c;" title="Amber"></button>
            <button type="button" class="w-6 h-6 rounded-full border-2 border-transparent hover:border-white/50 transition-all color-option-btn" data-color="#ef4444" style="background-color: #ef4444;" title="Red"></button>
            <button type="button" class="w-6 h-6 rounded-full border-2 border-transparent hover:border-white/50 transition-all color-option-btn" data-color="#f97316" style="background-color: #f97316;" title="Orange"></button>
            <button type="button" class="w-6 h-6 rounded-full border-2 border-transparent hover:border-white/50 transition-all color-option-btn" data-color="#10b981" style="background-color: #10b981;" title="Green"></button>
            <button type="button" class="w-6 h-6 rounded-full border-2 border-transparent hover:border-white/50 transition-all color-option-btn" data-color="#3b82f6" style="background-color: #3b82f6;" title="Blue"></button>
            <button type="button" class="w-6 h-6 rounded-full border-2 border-transparent hover:border-white/50 transition-all color-option-btn" data-color="#8b5cf6" style="background-color: #8b5cf6;" title="Purple"></button>
          </div>
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
  const noteInput = document.getElementById('bookmark-note-input');
  
  let selectedColor = '#e5a93c';
  const colorBtns = modal.querySelectorAll('.color-option-btn');
  colorBtns.forEach(btn => {
    btn.onclick = () => {
      colorBtns.forEach(b => {
        b.classList.remove('border-accent');
        b.classList.add('border-transparent');
      });
      btn.classList.remove('border-transparent');
      btn.classList.add('border-accent');
      selectedColor = btn.getAttribute('data-color');
    };
  });

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
        title: titleVal,
        note: noteInput.value.trim(),
        color: selectedColor
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
  const badge = document.getElementById('player-sleep-badge');
  if (badge) {
    badge.classList.remove('hidden');
    if (remaining > 60) {
      badge.textContent = `${Math.ceil(remaining / 60)}m`;
    } else {
      badge.textContent = `${remaining}s`;
    }
  }
  
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
  
  const badge = document.getElementById('player-sleep-badge');
  if (badge) {
    badge.classList.add('hidden');
  }
  
  const sleepIcon = document.getElementById('player-sleep-icon');
  if (sleepIcon) {
    sleepIcon.classList.remove('text-accent');
  }
  
  saveSleepTimerSettings();
}

function updateSleepTimerUI() {
  const sleepIcon = document.getElementById('player-sleep-icon');
  const badge = document.getElementById('player-sleep-badge');
  
  if (isSleepTimerActive) {
    if (sleepIcon) {
      sleepIcon.classList.add('text-accent');
    }
    if (badge) {
      badge.classList.remove('hidden');
      if (sleepTimerTimeRemaining > 60) {
        badge.textContent = `${Math.ceil(sleepTimerTimeRemaining / 60)}m`;
      } else {
        badge.textContent = `${sleepTimerTimeRemaining}s`;
      }
    }
  } else {
    if (sleepIcon) {
      sleepIcon.classList.remove('text-accent');
    }
    if (badge) {
      badge.classList.add('hidden');
    }
  }
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

function showNotification(message) {
  let container = document.getElementById('notification-container');
  if (!container) {
    container = document.createElement('div');
    container.id = 'notification-container';
    container.className = 'fixed bottom-24 right-6 z-50 space-y-2 pointer-events-none';
    document.body.appendChild(container);
  }
  const toast = document.createElement('div');
  toast.className = 'bg-accent text-primary font-bold px-4 py-2 rounded shadow-lg text-xs transition-all transform translate-y-10 opacity-0 duration-300 pointer-events-none';
  toast.textContent = message;
  container.appendChild(toast);
  
  setTimeout(() => {
    toast.classList.remove('translate-y-10', 'opacity-0');
  }, 10);
  
  setTimeout(() => {
    toast.classList.add('translate-y-10', 'opacity-0');
    setTimeout(() => toast.remove(), 300);
  }, 2500);
}

function triggerSleepTimerModal() {
  if (!currentItem) return;

  const chapters = currentItem?.media?.chapters || [];
  const currentSecs = audio ? audio.currentTime : 0;
  const activeChapter = chapters.find(c => currentSecs >= c.start && currentSecs < c.end);

  // Create Modal element using native dialog for light-dismiss support and semantic styling
  const dialog = document.createElement('dialog');
  dialog.id = 'sleep-timer-dialog';
  dialog.setAttribute('closedby', 'any');
  dialog.className = 'bg-primary border border-black-400 rounded-lg max-w-md w-full p-6 shadow-2xl space-y-4 focus:outline-none select-none text-white backdrop:bg-black-900/80 backdrop:backdrop-blur-sm open:flex open:flex-col open:items-stretch';
  dialog.innerHTML = `
    <div class="flex items-center justify-between border-b border-black-500 pb-3">
      <h3 class="text-sm font-bold text-white uppercase tracking-wider flex items-center space-x-1.5">
        <span class="material-symbols text-base text-accent">bedtime</span>
        <span>Sleep Timer</span>
      </h3>
      <button id="close-sleep-modal" class="text-black-100 hover:text-white transition-colors">
        <span class="material-symbols text-lg">close</span>
      </button>
    </div>

    <div class="space-y-4 text-left">
      <div>
        <label class="text-[0.65rem] uppercase font-bold text-black-100 tracking-wider block mb-2">Duration</label>
        <div class="grid grid-cols-3 gap-2">
          <button class="sleep-opt-btn bg-black-500 hover:bg-black-400 border border-black-300 text-white rounded py-2 text-xs transition-colors" data-value="off">Off</button>
          <button class="sleep-opt-btn bg-black-500 hover:bg-black-400 border border-black-300 text-white rounded py-2 text-xs transition-colors" data-value="5">5m</button>
          <button class="sleep-opt-btn bg-black-500 hover:bg-black-400 border border-black-300 text-white rounded py-2 text-xs transition-colors" data-value="15">15m</button>
          <button class="sleep-opt-btn bg-black-500 hover:bg-black-400 border border-black-300 text-white rounded py-2 text-xs transition-colors" data-value="30">30m</button>
          <button class="sleep-opt-btn bg-black-500 hover:bg-black-400 border border-black-300 text-white rounded py-2 text-xs transition-colors" data-value="45">45m</button>
          <button class="sleep-opt-btn bg-black-500 hover:bg-black-400 border border-black-300 text-white rounded py-2 text-xs transition-colors" data-value="60">60m</button>
        </div>
        ${activeChapter ? `
        <button class="sleep-opt-btn w-full mt-2 bg-black-500 hover:bg-black-400 border border-black-300 text-white rounded py-2 text-xs transition-colors flex items-center justify-center space-x-1" data-value="chapter">
          <span class="material-symbols text-sm">menu_book</span>
          <span>End of Chapter: ${escapeHtml(activeChapter.title || 'Current Chapter')}</span>
        </button>
        ` : ''}
      </div>

      <div class="space-y-3 pt-2">
        <label class="text-[0.65rem] uppercase font-bold text-black-100 tracking-wider block">Settings</label>
        
        <label class="flex items-center space-x-2.5 cursor-pointer select-none">
          <input type="checkbox" id="sleep-autorestart-input" class="rounded text-accent bg-black-500 border-black-300 focus:ring-0 focus:ring-offset-0">
          <div class="text-left">
            <span class="text-xs text-white font-medium block">Auto-Restart Timer on Play</span>
            <span class="text-[0.65rem] text-black-100">Automatically restart timer when resuming playback</span>
          </div>
        </label>

        <label class="flex items-center space-x-2.5 cursor-pointer select-none">
          <input type="checkbox" id="sleep-shaketoreset-input" class="rounded text-accent bg-black-500 border-black-300 focus:ring-0 focus:ring-offset-0">
          <div class="text-left">
            <span class="text-xs text-white font-medium block">Shake-to-Reset</span>
            <span class="text-[0.65rem] text-black-100">Shake device in the last 30s to extend/reset timer</span>
          </div>
        </label>
      </div>
    </div>

    <div class="flex items-center justify-end space-x-3 pt-3 border-t border-black-500">
      <button id="cancel-sleep-btn" class="bg-transparent hover:bg-black-500 text-white px-4 py-2 rounded text-xs transition-colors">
        Cancel
      </button>
      <button id="save-sleep-btn" class="bg-accent text-primary font-bold px-4 py-2 rounded text-xs hover:opacity-90 transition-opacity">
        Start Timer
      </button>
    </div>
  `;

  document.body.appendChild(dialog);

  // Set checkbox states from configuration variables
  const autoRestartInput = dialog.querySelector('#sleep-autorestart-input');
  const shakeToResetInput = dialog.querySelector('#sleep-shaketoreset-input');
  autoRestartInput.checked = sleepTimerAutoRestart;
  shakeToResetInput.checked = sleepTimerShakeToReset;

  let selectedValue = isSleepTimerActive ? sleepTimerType : 'off';
  const optButtons = dialog.querySelectorAll('.sleep-opt-btn');
  const highlightSelected = () => {
    optButtons.forEach(btn => {
      if (btn.getAttribute('data-value') === selectedValue) {
        btn.classList.add('border-accent', 'text-accent', 'bg-accent/10');
        btn.classList.remove('border-black-300', 'text-white');
      } else {
        btn.classList.remove('border-accent', 'text-accent', 'bg-accent/10');
        btn.classList.add('border-black-300', 'text-white');
      }
    });
  };

  optButtons.forEach(btn => {
    btn.onclick = () => {
      selectedValue = btn.getAttribute('data-value');
      highlightSelected();
    };
  });
  highlightSelected();

  const closeModal = () => {
    dialog.close();
    dialog.remove();
  };

  // Implement the fallback for light-dismiss on unsupported browsers (e.g. Safari)
  if (!('closedBy' in HTMLDialogElement.prototype)) {
    dialog.addEventListener('click', (event) => {
      if (event.target !== dialog) return;
      const rect = dialog.getBoundingClientRect();
      const isDialogContent = (
        rect.top <= event.clientY &&
        event.clientY <= rect.top + rect.height &&
        rect.left <= event.clientX &&
        event.clientX <= rect.left + rect.width
      );
      if (isDialogContent) return;
      closeModal();
    });
  }

  dialog.querySelector('#close-sleep-modal').onclick = closeModal;
  dialog.querySelector('#cancel-sleep-btn').onclick = closeModal;

  dialog.querySelector('#save-sleep-btn').onclick = () => {
    sleepTimerAutoRestart = autoRestartInput.checked;
    sleepTimerShakeToReset = shakeToResetInput.checked;
    
    if (selectedValue === 'off') {
      stopSleepTimer(true);
    } else {
      startSleepTimer(selectedValue);
    }
    closeModal();
  };

  dialog.showModal();
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

function drawWaveform() {
  const canvas = document.getElementById('player-waveform-canvas');
  if (!canvas) return;
  const ctx = canvas.getContext('2d');
  if (!ctx) return;

  const rect = canvas.getBoundingClientRect();
  if (rect.width === 0 || rect.height === 0) return;

  canvas.width = rect.width * window.devicePixelRatio;
  canvas.height = rect.height * window.devicePixelRatio;
  ctx.scale(window.devicePixelRatio, window.devicePixelRatio);

  const width = rect.width;
  const height = rect.height;

  ctx.clearRect(0, 0, width, height);

  if (!currentWaveform || currentWaveform.length === 0) {
    ctx.strokeStyle = '#2d2d2d';
    ctx.lineWidth = 2;
    ctx.beginPath();
    ctx.moveTo(0, height / 2);
    ctx.lineTo(width, height / 2);
    ctx.stroke();
    return;
  }

  const timeline = document.getElementById('player-timeline');
  const pct = timeline ? parseFloat(timeline.value) / 100 : 0;

  const barCount = currentWaveform.length;
  const gap = 2;
  const totalGapWidth = gap * (barCount - 1);
  const barWidth = (width - totalGapWidth) / barCount;

  for (let i = 0; i < barCount; i++) {
    const peak = currentWaveform[i];
    const barHeight = (peak / 255) * height * 0.8;
    const x = i * (barWidth + gap);
    const y = (height - barHeight) / 2;

    const barPct = i / barCount;
    const isPlayed = barPct <= pct;
    const isHoverPreview = hoverPct !== null && (
      (hoverPct >= pct && barPct > pct && barPct <= hoverPct) ||
      (hoverPct < pct && barPct > hoverPct && barPct <= pct)
    );

    if (isHoverPreview) {
      ctx.fillStyle = '#fcd34d'; // lighter amber preview seek region
    } else if (isPlayed) {
      ctx.fillStyle = '#f59e0b'; // played amber
    } else {
      ctx.fillStyle = '#4b5563'; // unplayed gray
    }

    drawRoundedRect(ctx, x, y, barWidth, barHeight, 1);
  }

  // Draw vertical seek cursor line on hover
  if (hoverX !== null) {
    ctx.strokeStyle = '#ffffff';
    ctx.lineWidth = 1.5;
    ctx.beginPath();
    ctx.moveTo(hoverX, 0);
    ctx.lineTo(hoverX, height);
    ctx.stroke();
  }
}

function drawRoundedRect(ctx, x, y, width, height, radius) {
  if (width <= 0 || height <= 0) return;
  if (radius > width / 2) radius = width / 2;
  if (radius > height / 2) radius = height / 2;
  ctx.beginPath();
  ctx.moveTo(x + radius, y);
  ctx.lineTo(x + width - radius, y);
  ctx.quadraticCurveTo(x + width, y, x + width, y + radius);
  ctx.lineTo(x + width, y + height - radius);
  ctx.quadraticCurveTo(x + width, y + height, x + width - radius, y + height);
  ctx.lineTo(x + radius, y + height);
  ctx.quadraticCurveTo(x, y + height, x, y + height - radius);
  ctx.lineTo(x, y + radius);
  ctx.quadraticCurveTo(x, y, x + radius, y);
  ctx.closePath();
  ctx.fill();
}

window.addEventListener('resize', drawWaveform);

function triggerPlayerSettingsModal() {
  const dialog = document.createElement('dialog');
  dialog.id = 'player-settings-dialog';
  dialog.setAttribute('closedby', 'any');
  dialog.className = 'bg-primary border border-black-400 rounded-lg max-w-md w-full p-6 shadow-2xl space-y-4 focus:outline-none select-none text-white backdrop:bg-black-900/80 backdrop:backdrop-blur-sm open:flex open:flex-col open:items-stretch';
  
  dialog.innerHTML = `
    <div class="flex items-center justify-between border-b border-black-500 pb-3">
      <h3 class="text-sm font-bold text-white uppercase tracking-wider flex items-center space-x-1.5">
        <span class="material-symbols text-base text-accent">settings</span>
        <span>Player Settings</span>
      </h3>
      <button id="close-settings-modal" class="text-black-100 hover:text-white transition-colors">
        <span class="material-symbols text-lg">close</span>
      </button>
    </div>

    <div class="space-y-4 text-left">
      <!-- Volume Boost -->
      <div class="space-y-2">
        <div class="flex justify-between items-center">
          <label class="text-[0.65rem] uppercase font-bold text-black-100 tracking-wider">Volume Boost</label>
          <span id="volume-boost-value" class="text-xs text-accent font-bold">1.0x (No Boost)</span>
        </div>
        <div class="flex items-center space-x-3">
          <span class="text-xs text-black-100">1.0x</span>
          <input type="range" id="volume-boost-slider" min="1.0" max="3.0" step="0.1" value="${volumeBoostLevel}" class="flex-grow accent-accent bg-black-500 h-1.5 rounded-lg cursor-pointer">
          <span class="text-xs text-black-100">3.0x</span>
        </div>
        <span class="text-[0.65rem] text-black-100 block">Boost audio levels beyond 100% using Web Audio API (recommended for quiet recordings).</span>
      </div>

      <hr class="border-black-500">

      <!-- Speed Settings -->
      <div class="space-y-3">
        <label class="text-[0.65rem] uppercase font-bold text-black-100 tracking-wider block">Playback Speed Settings</label>
        
        <label class="flex items-center space-x-2.5 cursor-pointer select-none">
          <input type="checkbox" id="speed-remember-input" class="rounded text-accent bg-black-500 border-black-300 focus:ring-0 focus:ring-offset-0">
          <div class="text-left">
            <span class="text-xs text-white font-medium block">Remember speed per book</span>
            <span class="text-[0.65rem] text-black-100">Automatically save and restore playback speed for each individual book</span>
          </div>
        </label>

        <div class="space-y-1">
          <label for="speed-default-select" class="text-xs text-white font-medium block">Default Playback Speed</label>
          <select id="speed-default-select" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent text-xs cursor-pointer">
            <option value="0.5">0.5x</option>
            <option value="0.75">0.75x</option>
            <option value="1.0">1.0x (Default)</option>
            <option value="1.25">1.25x</option>
            <option value="1.5">1.5x</option>
            <option value="1.75">1.75x</option>
            <option value="2.0">2.0x</option>
          </select>
        </div>
      </div>

      <hr class="border-black-500">

      <!-- Skip Durations -->
      <div class="space-y-3">
        <label class="text-[0.65rem] uppercase font-bold text-black-100 tracking-wider block">Skip Durations</label>
        
        <div class="grid grid-cols-2 gap-3">
          <div class="space-y-1">
            <label for="skip-back-select" class="text-xs text-white font-medium block">Jump Backward</label>
            <select id="skip-back-select" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent text-xs cursor-pointer">
              <option value="5">5s</option>
              <option value="10">10s</option>
              <option value="15">15s</option>
              <option value="30">30s</option>
              <option value="45">45s</option>
              <option value="60">60s</option>
            </select>
          </div>
          <div class="space-y-1">
            <label for="skip-forward-select" class="text-xs text-white font-medium block">Jump Forward</label>
            <select id="skip-forward-select" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent text-xs cursor-pointer">
              <option value="5">5s</option>
              <option value="10">10s</option>
              <option value="15">15s</option>
              <option value="30">30s</option>
              <option value="45">45s</option>
              <option value="60">60s</option>
            </select>
          </div>
        </div>
      </div>
    </div>

    <div class="flex items-center justify-end space-x-3 pt-3 border-t border-black-500">
      <button id="cancel-settings-btn" class="bg-transparent hover:bg-black-500 text-white px-4 py-2 rounded text-xs transition-colors">
        Cancel
      </button>
      <button id="save-settings-btn" class="bg-accent text-primary font-bold px-4 py-2 rounded text-xs hover:opacity-90 transition-opacity">
        Save Settings
      </button>
    </div>
  `;

  document.body.appendChild(dialog);

  const rememberInput = dialog.querySelector('#speed-remember-input');
  const defaultSelect = dialog.querySelector('#speed-default-select');
  const boostSlider = dialog.querySelector('#volume-boost-slider');
  const boostValSpan = dialog.querySelector('#volume-boost-value');
  const skipBackSelect = dialog.querySelector('#skip-back-select');
  const skipForwardSelect = dialog.querySelector('#skip-forward-select');

  rememberInput.checked = rememberSpeedPerBook;
  defaultSelect.value = globalDefaultSpeed.toString();
  boostSlider.value = volumeBoostLevel;
  skipBackSelect.value = playerSkipBackSeconds.toString();
  skipForwardSelect.value = playerSkipForwardSeconds.toString();

  const updateBoostLabel = (val) => {
    const v = parseFloat(val);
    if (v === 1.0) {
      boostValSpan.textContent = '1.0x (No Boost)';
    } else {
      const db = (20 * Math.log10(v)).toFixed(1);
      boostValSpan.textContent = `${v.toFixed(1)}x (+${db} dB)`;
    }
  };
  updateBoostLabel(volumeBoostLevel);

  boostSlider.oninput = () => {
    updateBoostLabel(boostSlider.value);
  };

  const closeModal = () => {
    dialog.close();
    dialog.remove();
  };

  if (!('closedBy' in HTMLDialogElement.prototype)) {
    dialog.addEventListener('click', (event) => {
      if (event.target !== dialog) return;
      const rect = dialog.getBoundingClientRect();
      const isDialogContent = (
        rect.top <= event.clientY &&
        event.clientY <= rect.top + rect.height &&
        rect.left <= event.clientX &&
        event.clientX <= rect.left + rect.width
      );
      if (isDialogContent) return;
      closeModal();
    });
  }

  dialog.querySelector('#close-settings-modal').onclick = closeModal;
  dialog.querySelector('#cancel-settings-btn').onclick = closeModal;

  dialog.querySelector('#save-settings-btn').onclick = () => {
    rememberSpeedPerBook = rememberInput.checked;
    globalDefaultSpeed = parseFloat(defaultSelect.value) || 1.0;
    volumeBoostLevel = parseFloat(boostSlider.value) || 1.0;
    playerSkipBackSeconds = parseInt(skipBackSelect.value, 10) || 10;
    playerSkipForwardSeconds = parseInt(skipForwardSelect.value, 10) || 10;

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

    updateSkipButtonsUI();
    closeModal();
  };

  dialog.showModal();
}

export async function addToQueue(item) {
  let itemObj = item;
  if (typeof item === 'string') {
    try {
      itemObj = await request('GET', `/api/items/${item}`);
    } catch (err) {
      console.error('Failed to fetch item to queue:', err);
      showNotification('Failed to add item to queue');
      return;
    }
  }

  // Prevent duplicate items in queue
  if (playbackQueue.some(q => q.id === itemObj.id)) {
    showNotification('Item is already in queue');
    return;
  }

  playbackQueue.push(itemObj);
  updateQueueUI();
  
  const title = itemObj.media?.metadata?.title || itemObj.title || 'Item';
  showNotification(`Added "${title}" to queue`);
}

export function getQueue() {
  return playbackQueue;
}

export function clearQueue() {
  playbackQueue = [];
  updateQueueUI();
  showNotification('Queue cleared');
}

export function removeFromQueue(index) {
  if (index >= 0 && index < playbackQueue.length) {
    const removed = playbackQueue.splice(index, 1)[0];
    updateQueueUI();
    const title = removed.media?.metadata?.title || removed.title || 'Item';
    showNotification(`Removed "${title}" from queue`);
  }
}

export function reorderQueue(fromIndex, toIndex) {
  if (fromIndex >= 0 && fromIndex < playbackQueue.length && toIndex >= 0 && toIndex < playbackQueue.length) {
    const element = playbackQueue.splice(fromIndex, 1)[0];
    playbackQueue.splice(toIndex, 0, element);
    updateQueueUI();
  }
}

function updateQueueUI() {
  const badge = document.getElementById('player-queue-badge');
  if (badge) {
    if (playbackQueue.length > 0) {
      badge.textContent = playbackQueue.length;
      badge.classList.remove('hidden');
    } else {
      badge.classList.add('hidden');
    }
  }

  // If queue modal is open, re-render its content
  const dialog = document.getElementById('player-queue-dialog');
  if (dialog && dialog.open) {
    renderQueueDialogContent(dialog);
  }
}

function renderQueueDialogContent(dialog) {
  const queueListContainer = dialog.querySelector('#queue-list-container');
  if (!queueListContainer) return;

  if (playbackQueue.length === 0) {
    queueListContainer.innerHTML = `
      <div class="text-center py-8 text-black-100 text-sm">
        <span class="material-symbols text-4xl block mb-2 opacity-40">queue_music</span>
        <span>Queue is empty</span>
      </div>
    `;
    const clearBtn = dialog.querySelector('#clear-queue-btn');
    if (clearBtn) clearBtn.disabled = true;
    return;
  }

  const clearBtn = dialog.querySelector('#clear-queue-btn');
  if (clearBtn) clearBtn.disabled = false;

  const token = localStorage.getItem('token');
  
  queueListContainer.innerHTML = '';
  playbackQueue.forEach((item, idx) => {
    const ts = item.updatedAt || item.addedAt || Date.now();
    const coverUrl = resolvePath(`/api/items/${item.id}/cover?token=${token}&ts=${ts}`);
    
    let title = 'Untitled';
    let author = 'Unknown';
    let durationStr = 'N/A';
    
    if (item.mediaType === 'book') {
      const metadata = item.media?.metadata || {};
      title = metadata.title || item.title || 'Untitled';
      author = metadata.authorName || 'Unknown';
      const durSec = item.media?.duration || 0;
      durationStr = durSec ? formatTime(durSec) : 'N/A';
    } else if (item.mediaType === 'podcast') {
      const metadata = item.media?.metadata || {};
      title = metadata.title || item.title || 'Untitled';
      author = metadata.author || 'Unknown';
      const durSec = item.media?.duration || 0;
      durationStr = durSec ? formatTime(durSec) : 'N/A';
    }

    const row = document.createElement('div');
    row.className = 'queue-item-row flex items-center space-x-3 p-2 bg-black-500/40 rounded border border-black-400/30 hover:border-black-400 transition-all cursor-move select-none';
    row.setAttribute('draggable', 'true');
    row.dataset.index = idx;
    row.innerHTML = `
      <div class="flex items-center text-black-200 hover:text-white select-none mr-1 drag-handle cursor-grab active:cursor-grabbing">
        <span class="material-symbols text-lg">drag_handle</span>
      </div>
      <img src="${coverUrl}" class="w-8 h-12 object-cover rounded shadow border border-black-400/20" onerror="this.onerror=null; this.src='assets/images/logo.png'">
      <div class="flex-grow min-w-0 text-left">
        <div class="text-xs font-semibold text-white truncate">${escapeHtml(title)}</div>
        <div class="text-[10px] text-black-50 truncate">${escapeHtml(author)}</div>
        <div class="text-[10px] text-accent font-mono mt-0.5">${durationStr}</div>
      </div>
      <div class="flex items-center space-x-1">
        <button class="move-up-btn p-1 hover:bg-black-400 rounded text-black-50 hover:text-white transition-colors" data-index="${idx}" title="Move Up">
          <span class="material-symbols text-base">arrow_upward</span>
        </button>
        <button class="move-down-btn p-1 hover:bg-black-400 rounded text-black-50 hover:text-white transition-colors" data-index="${idx}" title="Move Down">
          <span class="material-symbols text-base">arrow_downward</span>
        </button>
        <button class="remove-btn p-1 hover:bg-red-900/60 rounded text-black-50 hover:text-red-400 transition-colors" data-index="${idx}" title="Remove">
          <span class="material-symbols text-base">delete</span>
        </button>
      </div>
    `;

    // Hook events
    row.querySelector('.move-up-btn').onclick = (e) => {
      e.stopPropagation();
      reorderQueue(idx, idx - 1);
    };
    row.querySelector('.move-down-btn').onclick = (e) => {
      e.stopPropagation();
      reorderQueue(idx, idx + 1);
    };
    row.querySelector('.remove-btn').onclick = (e) => {
      e.stopPropagation();
      removeFromQueue(idx);
    };

    // Attach HTML5 drag and drop events
    row.addEventListener('dragstart', (e) => {
      draggedQueueIndex = idx;
      row.classList.add('opacity-40');
      e.dataTransfer.effectAllowed = 'move';
    });

    row.addEventListener('dragend', () => {
      row.classList.remove('opacity-40');
      draggedQueueIndex = null;
    });

    row.addEventListener('dragover', (e) => {
      e.preventDefault();
      e.dataTransfer.dropEffect = 'move';
    });

    row.addEventListener('dragenter', () => {
      if (draggedQueueIndex !== null && idx !== draggedQueueIndex) {
        row.classList.add('bg-black-400/80');
      }
    });

    row.addEventListener('dragleave', () => {
      row.classList.remove('bg-black-400/80');
    });

    row.addEventListener('drop', (e) => {
      e.preventDefault();
      row.classList.remove('bg-black-400/80');
      if (draggedQueueIndex !== null && draggedQueueIndex !== idx) {
        reorderQueue(draggedQueueIndex, idx);
      }
    });

    queueListContainer.appendChild(row);
  });
}

function triggerQueueModal() {
  let dialog = document.getElementById('player-queue-dialog');
  if (dialog) {
    dialog.remove();
  }

  dialog = document.createElement('dialog');
  dialog.id = 'player-queue-dialog';
  dialog.setAttribute('closedby', 'any');
  dialog.className = 'bg-primary border border-black-400 rounded-lg max-w-md w-full p-6 shadow-2xl space-y-4 focus:outline-none select-none text-white backdrop:bg-black-900/80 backdrop:backdrop-blur-sm open:flex open:flex-col open:items-stretch';

  dialog.innerHTML = `
    <div class="flex items-center justify-between border-b border-black-500 pb-3">
      <h3 class="text-sm font-bold text-white uppercase tracking-wider flex items-center space-x-1.5">
        <span class="material-symbols text-base text-accent">queue_music</span>
        <span>Playback Queue</span>
      </h3>
      <button id="close-queue-modal" class="text-black-100 hover:text-white transition-colors">
        <span class="material-symbols text-lg">close</span>
      </button>
    </div>

    <div id="queue-list-container" class="max-h-[300px] overflow-y-auto space-y-2 pr-1 no-scroll">
      <!-- Queue items dynamically injected -->
    </div>

    <div class="flex items-center justify-between pt-3 border-t border-black-500">
      <button id="clear-queue-btn" class="bg-red-600 hover:bg-red-700 text-white px-3 py-1.5 rounded text-xs transition-colors flex items-center space-x-1">
        <span class="material-symbols text-sm">clear_all</span>
        <span>Clear Queue</span>
      </button>
      <button id="close-queue-btn" class="bg-black-500 hover:bg-black-400 text-white px-4 py-1.5 rounded text-xs transition-colors">
        Close
      </button>
    </div>
  `;

  document.body.appendChild(dialog);

  // Initial render
  renderQueueDialogContent(dialog);

  const closeModal = () => {
    dialog.close();
    dialog.remove();
  };

  if (!('closedBy' in HTMLDialogElement.prototype)) {
    dialog.addEventListener('click', (event) => {
      if (event.target !== dialog) return;
      const rect = dialog.getBoundingClientRect();
      const isDialogContent = (
        rect.top <= event.clientY &&
        event.clientY <= rect.top + rect.height &&
        rect.left <= event.clientX &&
        event.clientX <= rect.left + rect.width
      );
      if (isDialogContent) return;
      closeModal();
    });
  }

  dialog.querySelector('#close-queue-modal').onclick = closeModal;
  dialog.querySelector('#close-queue-btn').onclick = closeModal;
  dialog.querySelector('#clear-queue-btn').onclick = () => {
    clearQueue();
  };

  dialog.showModal();
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


