import { request, resolvePath } from '../api.js';
import { isCasting, getRemotePlayer } from './cast.js';
import { getQueue, getQueueLength, triggerQueueModal, updateQueueUI } from './queue.js';

export let playerController = null;

export function setPlayerController(c) {
  playerController = c;
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

export function formatTime(secs) {
  if (isNaN(secs) || secs === Infinity || secs === null || secs === undefined) return '0:00';
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

export function updateTimelineUI() {
  if (!playerController) return;
  const currentItem = playerController.getCurrentItem();
  const audio = playerController.getAudio();

  let elapsed = 0;
  let duration = 0;
  
  if (isCasting()) {
    const remotePlayer = getRemotePlayer();
    elapsed = remotePlayer.currentTime || 0;
    duration = remotePlayer.duration || 0;
  } else {
    if (!audio || !audio.duration) return;
    elapsed = audio.currentTime || 0;
    duration = audio.duration;
  }
  
  const elapsedStr = formatTime(elapsed);
  const remainingStr = formatTime(duration - elapsed);
  const timelineVal = duration > 0 ? Math.round((elapsed / duration) * 100) : 0;

  if ('mediaSession' in navigator && 'setPositionState' in navigator.mediaSession) {
    try {
      if (duration > 0 && elapsed >= 0 && elapsed <= duration) {
        const rate = isCasting() ? 1.0 : (audio ? audio.playbackRate : 1.0);
        navigator.mediaSession.setPositionState({
          duration: duration,
          playbackRate: rate,
          position: elapsed
        });
      }
    } catch (err) {
      console.warn('Failed to update MediaSession position state:', err);
    }
  }

  ['player-time-elapsed', 'expanded-time-elapsed'].forEach(id => {
    const el = document.getElementById(id);
    if (el) el.textContent = elapsedStr;
  });
  ['player-time-remaining', 'expanded-time-remaining'].forEach(id => {
    const el = document.getElementById(id);
    if (el) el.textContent = remainingStr;
  });
  ['player-timeline', 'expanded-timeline'].forEach(id => {
    const el = document.getElementById(id);
    if (el) el.value = timelineVal;
  });

  drawWaveform();

  const chapters = currentItem?.media?.chapters || [];
  const activeChapter = chapters.find(c => elapsed >= c.start && elapsed < c.end);
  const infoText = activeChapter 
    ? `Chapter ${chapters.indexOf(activeChapter) + 1} of ${chapters.length}: ${activeChapter.title || 'Untitled'}`
    : '';

  ['player-chapter-info', 'expanded-chapter-info'].forEach(id => {
    const el = document.getElementById(id);
    if (el) {
      if (infoText) {
        el.textContent = infoText;
        el.classList.remove('hidden');
      } else {
        el.classList.add('hidden');
      }
    }
  });

  updatePlaybackControlsUI();
}

export function updatePlayPauseButton(isPlaying) {
  ['player-play-pause-icon', 'player-play-pause-icon-mobile', 'expanded-play-pause-icon'].forEach(id => {
    const el = document.getElementById(id);
    if (el) el.textContent = isPlaying ? 'pause' : 'play_arrow';
  });
}

export function updateVolumeIcon(vol) {
  ['player-volume-icon', 'expanded-volume-icon'].forEach(id => {
    const el = document.getElementById(id);
    if (!el) return;
    if (vol === 0) {
      el.textContent = 'volume_mute';
    } else if (vol < 0.5) {
      el.textContent = 'volume_down';
    } else {
      el.textContent = 'volume_up';
    }
  });
}

export function updateMetadataUI(item) {
  if (!item) return;
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

  const token = localStorage.getItem('token');
  const ts = item.updatedAt || item.addedAt || Date.now();
  const coverUrl = resolvePath(`/api/items/${item.id}/cover?token=${token}&ts=${ts}`);

  const titleEl = document.getElementById('player-title');
  const authorEl = document.getElementById('player-author');
  const coverEl = document.getElementById('player-cover');
  if (titleEl) titleEl.textContent = title;
  if (authorEl) authorEl.textContent = author;
  if (coverEl) coverEl.src = coverUrl;

  if ('mediaSession' in navigator) {
    try {
      navigator.mediaSession.metadata = new MediaMetadata({
        title: title,
        artist: author,
        album: item.mediaType === 'book' ? (item.media?.metadata?.seriesName || 'Audiobook') : 'Podcast',
        artwork: [
          { src: coverUrl, sizes: '512x512', type: 'image/jpeg' }
        ]
      });
    } catch (err) {
      console.warn('Failed to update MediaSession metadata:', err);
    }
  }

  const expandedDialog = document.getElementById('expanded-player-dialog');
  if (expandedDialog) {
    const expTitle = expandedDialog.querySelector('h2');
    const expAuthor = expandedDialog.querySelector('p');
    const expCover = expandedDialog.querySelector('img');
    if (expTitle) expTitle.textContent = title;
    if (expAuthor) expAuthor.textContent = author;
    if (expCover) expCover.src = coverUrl;
  }

  const chaptersBtn = document.getElementById('player-chapters-btn');
  const expChaptersBtn = document.getElementById('expanded-chapters-btn');
  const chapters = item.media?.chapters || [];

  [chaptersBtn, expChaptersBtn].forEach(btn => {
    if (btn) {
      if (chapters.length > 0) {
        btn.classList.remove('hidden');
        btn.classList.remove('opacity-40');
        btn.classList.remove('pointer-events-none');
      } else {
        btn.classList.add('hidden');
        btn.classList.add('opacity-40');
        btn.classList.add('pointer-events-none');
      }
    }
  });

  const nextBtn = document.getElementById('player-next');
  const expNextBtn = document.getElementById('expanded-next');
  [nextBtn, expNextBtn].forEach(btn => {
    if (btn) {
      if (chapters.length > 0 || getQueueLength() > 0) {
        btn.classList.remove('hidden');
      } else {
        btn.classList.add('hidden');
      }
    }
  });

  const prevBtn = document.getElementById('player-prev-chapter');
  const expPrevBtn = document.getElementById('expanded-prev-chapter');
  [prevBtn, expPrevBtn].forEach(btn => {
    if (btn) {
      if (chapters.length > 0) {
        btn.classList.remove('hidden');
      } else {
        btn.classList.add('hidden');
      }
    }
  });

  updatePlaybackControlsUI();
  updateSkipButtonsUI();
}

export function updateSkipButtonsUI() {
  if (!playerController) return;
  const skipBack = playerController.getPlayerSkipBackSeconds();
  const skipForward = playerController.getPlayerSkipForwardSeconds();

  const seekBacks = [
    document.getElementById('player-seek-back'),
    document.getElementById('expanded-seek-back')
  ].filter(Boolean);

  seekBacks.forEach(btn => {
    btn.title = `Seek Back ${skipBack}s`;
    const icon = btn.querySelector('.material-symbols');
    if (icon) {
      if ([5, 10, 15, 30, 45, 60].includes(skipBack)) {
        icon.textContent = `replay_${skipBack}`;
      } else {
        icon.textContent = 'replay';
      }
    }
  });

  const seekForwards = [
    document.getElementById('player-seek-forward'),
    document.getElementById('expanded-seek-forward')
  ].filter(Boolean);

  seekForwards.forEach(btn => {
    btn.title = `Seek Forward ${skipForward}s`;
    const icon = btn.querySelector('.material-symbols');
    if (icon) {
      if ([5, 10, 15, 30, 45, 60].includes(skipForward)) {
        icon.textContent = `forward_${skipForward}`;
      } else {
        icon.textContent = 'forward_media';
      }
    }
  });
}

export function updatePlaybackControlsUI() {
  if (!playerController) return;
  const currentItem = playerController.getCurrentItem();
  const prevChapterBtn = document.getElementById('player-prev-chapter');
  const nextBtn = document.getElementById('player-next');
  if (!prevChapterBtn || !nextBtn) return;

  const chapters = currentItem?.media?.chapters || [];
  
  if (chapters.length > 0) {
    prevChapterBtn.classList.remove('hidden');
    nextBtn.classList.remove('hidden');
  } else if (getQueueLength() > 0) {
    prevChapterBtn.classList.add('hidden');
    nextBtn.classList.remove('hidden');
  } else {
    prevChapterBtn.classList.add('hidden');
    nextBtn.classList.add('hidden');
  }
}

export function triggerChaptersModal() {
  if (!playerController) return;
  const currentItem = playerController.getCurrentItem();
  if (!currentItem || !currentItem.media || !currentItem.media.chapters) return;
  const chapters = currentItem.media.chapters;
  if (chapters.length === 0) return;

  // Modal element
  const dialog = document.createElement('dialog');
  dialog.id = 'player-chapters-dialog';
  dialog.setAttribute('closedby', 'any');
  dialog.className = 'bg-primary border border-black-400 rounded-lg max-w-sm w-full p-6 shadow-2xl space-y-4 focus:outline-none select-none text-white backdrop:bg-black-900/80 backdrop:backdrop-blur-sm open:flex open:flex-col open:items-stretch';
  dialog.innerHTML = `
    <div class="flex items-center justify-between border-b border-black-500 pb-3">
      <h3 class="text-sm font-bold text-white uppercase tracking-wider flex items-center space-x-1.5">
        <span class="material-symbols text-base text-accent">format_list_bulleted</span>
        <span>Chapters</span>
      </h3>
      <button id="close-chapters-modal" class="text-black-100 hover:text-white transition-colors">
        <span class="material-symbols text-lg">close</span>
      </button>
    </div>

    <div class="max-h-[300px] overflow-y-auto space-y-1.5 pr-1" id="chapters-list-container">
      ${chapters.map((ch, idx) => `
        <button class="chapter-row-btn w-full text-left p-2 rounded text-xs hover:bg-black-500/50 transition-colors flex justify-between items-center" data-start="${ch.start}">
          <span class="font-medium text-white truncate max-w-[80%]">${escapeHtml(ch.title || `Chapter ${idx + 1}`)}</span>
          <span class="font-mono text-black-50 text-[10px]">${formatTime(ch.start)}</span>
        </button>
      `).join('')}
    </div>
  `;

  document.body.appendChild(dialog);

  const currentSecs = isCasting() ? (getRemotePlayer().currentTime || 0) : (playerController.getAudio() ? playerController.getAudio().currentTime : 0);
  const activeChapter = chapters.find(c => currentSecs >= c.start && currentSecs < c.end);
  if (activeChapter) {
    const idx = chapters.indexOf(activeChapter);
    const btns = dialog.querySelectorAll('.chapter-row-btn');
    if (btns[idx]) {
      btns[idx].classList.add('bg-accent/15', 'border', 'border-accent/30', 'text-accent');
      btns[idx].querySelector('span').classList.replace('text-white', 'text-accent');
    }
  }

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

  dialog.querySelectorAll('.chapter-row-btn').forEach(btn => {
    btn.onclick = () => {
      const start = parseFloat(btn.dataset.start);
      playerController.seekTo(start);
      closeModal();
    };
  });

  dialog.showModal();
}

export function triggerAddBookmarkModal() {
  if (!playerController) return;
  const currentItem = playerController.getCurrentItem();
  if (!currentItem) return;
  
  let time = 0;
  if (isCasting()) {
    time = getRemotePlayer().currentTime || 0;
  } else {
    const audio = playerController.getAudio();
    if (!audio) return;
    time = audio.currentTime || 0;
  }
  
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
      showToast("Title is required", "warning");
      return;
    }

    try {
      await playerController.addBookmark(time, titleVal, noteInput.value.trim(), selectedColor);
      closeModal();
    } catch (err) {
      console.error('Failed to create bookmark:', err);
      showToast('Failed to save bookmark: ' + (err.message || 'Unknown error'), 'error');
    }
  };
}

export function triggerSleepTimerModal() {
  if (!playerController) return;
  const currentItem = playerController.getCurrentItem();
  if (!currentItem) return;

  const chapters = currentItem?.media?.chapters || [];
  const currentSecs = playerController.getAudio() ? playerController.getAudio().currentTime : 0;
  const activeChapter = chapters.find(c => currentSecs >= c.start && currentSecs < c.end);

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

  const autoRestartInput = dialog.querySelector('#sleep-autorestart-input');
  const shakeToResetInput = dialog.querySelector('#sleep-shaketoreset-input');
  autoRestartInput.checked = playerController.getSleepTimerAutoRestart();
  shakeToResetInput.checked = playerController.getSleepTimerShakeToReset();

  let selectedValue = playerController.getSleepTimerType();
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
    const autoRestart = autoRestartInput.checked;
    const shakeToReset = shakeToResetInput.checked;
    
    playerController.setSleepTimerAutoRestart(autoRestart);
    playerController.setSleepTimerShakeToReset(shakeToReset);
    
    if (selectedValue === 'off') {
      playerController.stopSleepTimer(true);
    } else {
      playerController.startSleepTimer(selectedValue, autoRestart, shakeToReset);
    }
    closeModal();
  };

  dialog.showModal();
}

export function triggerPlayerSettingsModal() {
  if (!playerController) return;
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
          <input type="range" id="volume-boost-slider" min="1.0" max="3.0" step="0.1" value="${playerController.getVolumeBoostLevel()}" class="flex-grow accent-accent bg-black-500 h-1.5 rounded-lg cursor-pointer">
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

  rememberInput.checked = playerController.getRememberSpeedPerBook();
  defaultSelect.value = playerController.getGlobalDefaultSpeed().toString();
  boostSlider.value = playerController.getVolumeBoostLevel();
  skipBackSelect.value = playerController.getPlayerSkipBackSeconds().toString();
  skipForwardSelect.value = playerController.getPlayerSkipForwardSeconds().toString();

  const updateBoostLabel = (val) => {
    const v = parseFloat(val);
    if (v === 1.0) {
      boostValSpan.textContent = '1.0x (No Boost)';
    } else {
      const db = (20 * Math.log10(v)).toFixed(1);
      boostValSpan.textContent = `${v.toFixed(1)}x (+${db} dB)`;
    }
  };
  updateBoostLabel(playerController.getVolumeBoostLevel());

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
    const remember = rememberInput.checked;
    const defaultSpeed = parseFloat(defaultSelect.value) || 1.0;
    const volumeBoost = parseFloat(boostSlider.value) || 1.0;
    const skipBackVal = parseInt(skipBackSelect.value, 10) || 10;
    const skipForwardVal = parseInt(skipForwardSelect.value, 10) || 10;

    playerController.saveSettings({
      rememberSpeedPerBook: remember,
      globalDefaultSpeed: defaultSpeed,
      volumeBoostLevel: volumeBoost,
      playerSkipBackSeconds: skipBackVal,
      playerSkipForwardSeconds: skipForwardVal
    });

    updateSkipButtonsUI();
    closeModal();
  };

  dialog.showModal();
}

export function drawWaveform() {
  if (!playerController) return;
  const currentWaveform = playerController.getCurrentWaveform();
  const hoverPct = playerController.getHoverPct();

  const canvasIds = ['player-waveform-canvas', 'expanded-waveform-canvas'];
  canvasIds.forEach(canvasId => {
    const canvas = document.getElementById(canvasId);
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
      const timelineId = canvasId === 'player-waveform-canvas' ? 'player-timeline' : 'expanded-timeline';
      const timeline = document.getElementById(timelineId);
      const pct = timeline ? parseFloat(timeline.value) / 100 : 0;
      
      const trackHeight = 4;
      const y = (height - trackHeight) / 2;

      // Draw background track
      ctx.fillStyle = '#374151';
      drawRoundedRect(ctx, 0, y, width, trackHeight, trackHeight / 2);

      // Draw played track
      ctx.fillStyle = '#e5a93b';
      drawRoundedRect(ctx, 0, y, pct * width, trackHeight, trackHeight / 2);

      // Draw hover preview track if hovering
      if (hoverPct !== null) {
        ctx.fillStyle = '#fcd34d';
        if (hoverPct > pct) {
          drawRoundedRect(ctx, pct * width, y, (hoverPct - pct) * width, trackHeight, trackHeight / 2);
        } else if (hoverPct < pct) {
          drawRoundedRect(ctx, hoverPct * width, y, (pct - hoverPct) * width, trackHeight, trackHeight / 2);
        }

        // Draw vertical hover indicator line
        const canvasHoverX = hoverPct * width;
        ctx.strokeStyle = '#ffffff';
        ctx.lineWidth = 1.5;
        ctx.beginPath();
        ctx.moveTo(canvasHoverX, 0);
        ctx.lineTo(canvasHoverX, height);
        ctx.stroke();
      }
      return;
    }

    const timelineId = canvasId === 'player-waveform-canvas' ? 'player-timeline' : 'expanded-timeline';
    const timeline = document.getElementById(timelineId);
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
        ctx.fillStyle = '#fcd34d';
      } else if (isPlayed) {
        ctx.fillStyle = '#f59e0b';
      } else {
        ctx.fillStyle = '#4b5563';
      }

      drawRoundedRect(ctx, x, y, barWidth, barHeight, 1);
    }

    if (hoverPct !== null) {
      const canvasHoverX = hoverPct * width;
      ctx.strokeStyle = '#ffffff';
      ctx.lineWidth = 1.5;
      ctx.beginPath();
      ctx.moveTo(canvasHoverX, 0);
      ctx.lineTo(canvasHoverX, height);
      ctx.stroke();
    }
  });
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

export function showNotification(message) {
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

export function updateSleepTimerUI() {
  if (!playerController) return;
  const isSleepTimerActive = playerController.getSleepTimerTimeRemaining() > 0;
  const sleepTimerTimeRemaining = playerController.getSleepTimerTimeRemaining();

  ['player-sleep-badge', 'expanded-sleep-badge'].forEach(id => {
    const badge = document.getElementById(id);
    if (badge) {
      if (isSleepTimerActive) {
        let mins = Math.floor(sleepTimerTimeRemaining / 60);
        let secs = sleepTimerTimeRemaining % 60;
        badge.textContent = `${mins}m`;
        badge.classList.remove('hidden');
        badge.title = `Sleep Timer: ${mins}:${secs < 10 ? '0' : ''}${secs} remaining`;
      } else {
        badge.classList.add('hidden');
      }
    }
  });

  const sleepIcon = document.getElementById('player-sleep-icon');
  const expSleepIcon = document.getElementById('expanded-sleep-icon');
  [sleepIcon, expSleepIcon].forEach(icon => {
    if (icon) {
      if (isSleepTimerActive) {
        icon.classList.add('text-accent');
      } else {
        icon.classList.remove('text-accent');
      }
    }
  });
}

export function triggerExpandedPlayer() {
  if (!playerController) return;
  const currentItem = playerController.getCurrentItem();
  const audio = playerController.getAudio();
  if (!currentItem) return;

  if (document.getElementById('expanded-player-dialog')) return;

  const dialog = document.createElement('dialog');
  dialog.id = 'expanded-player-dialog';
  dialog.className = 'fixed inset-0 w-full h-full bg-primary flex flex-col p-4 sm:p-6 text-white z-50 overflow-hidden select-none max-w-none max-h-none border-none outline-none';

  const token = localStorage.getItem('token');
  const ts = currentItem.updatedAt || currentItem.addedAt || Date.now();
  const coverUrl = resolvePath(`/api/items/${currentItem.id}/cover?token=${token}&ts=${ts}`);

  let title = '';
  let author = '';
  if (currentItem.mediaType === 'book') {
    const metadata = currentItem.media?.metadata || {};
    title = metadata.title || currentItem.title || 'Untitled';
    author = metadata.authorName || 'Unknown';
  } else if (currentItem.mediaType === 'podcast') {
    const metadata = currentItem.media?.metadata || {};
    title = metadata.title || currentItem.title || 'Untitled';
    author = metadata.author || 'Unknown';
  } else {
    title = currentItem.title || 'Untitled';
    author = 'Unknown';
  }

  const speedOptions = [0.5, 0.75, 1.0, 1.25, 1.5, 1.75, 2.0].map(s => {
    const currentRate = audio ? audio.playbackRate : 1.0;
    return `<option value="${s}" ${currentRate === s ? 'selected' : ''}>${s}x</option>`;
  }).join('');

  dialog.innerHTML = `
    <!-- Header -->
    <div class="flex items-center justify-between border-b border-black-600/50 pb-3 sm:pb-4 flex-shrink-0 w-full max-w-3xl mx-auto">
      <button id="expanded-close-btn" class="p-2 hover:bg-black-500 rounded text-black-50 hover:text-white flex items-center justify-center transition-all" title="Minimize">
        <span class="material-symbols text-2xl">keyboard_arrow_down</span>
      </button>
      <span class="text-xs uppercase font-bold text-black-100 tracking-wider">Now Playing</span>
      <div class="flex items-center space-x-2">
        <button id="expanded-settings-btn" class="p-2 hover:bg-black-500 rounded text-black-50 hover:text-white flex items-center justify-center transition-all" title="Player Settings">
          <span class="material-symbols text-xl">settings</span>
        </button>
      </div>
    </div>

    <!-- Main Content Container -->
    <div class="flex-grow flex flex-col items-center justify-center py-4 sm:py-6 w-full max-w-xl mx-auto overflow-y-auto no-scroll space-y-4 sm:space-y-6">
      <!-- Large Cover Image -->
      <div class="w-48 h-48 sm:w-64 sm:h-64 md:w-80 md:h-80 bg-black-500 rounded-lg shadow-2xl border border-black-400 overflow-hidden flex-shrink-0 relative group max-h-[30vh] max-w-[30vh] aspect-square">
        <img src="${coverUrl}" alt="${title}" class="w-full h-full object-cover">
        ${currentItem.mediaType === 'book' ? '<div class="book-spine-crease"></div>' : ''}
      </div>

      <!-- Title / Author Info -->
      <div class="text-center px-4 w-full flex-shrink-0 space-y-1">
        <h2 class="text-lg sm:text-xl font-bold text-white tracking-wide truncate max-w-full" title="${title}">${title}</h2>
        <p class="text-xs sm:text-sm text-black-50 truncate max-w-full" title="${author}">${author}</p>
        <div id="expanded-chapter-info" class="text-[10px] sm:text-xs text-accent font-semibold truncate hidden"></div>
      </div>
    </div>

    <!-- Control & Progress Section -->
    <div class="w-full max-w-xl mx-auto pb-4 sm:pb-6 space-y-4 sm:space-y-5 flex-shrink-0">
      <!-- Timeline and Progress -->
      <div class="flex flex-col space-y-1">
        <div class="flex items-center w-full space-x-2 text-[10px] sm:text-xs text-black-50">
          <span id="expanded-time-elapsed" class="min-w-[35px] sm:min-w-[40px]">0:00</span>
          <div class="flex-grow relative h-6 flex items-center">
            <div id="expanded-waveform-container" class="absolute inset-0 w-full h-full pointer-events-none flex items-center z-0">
              <canvas id="expanded-waveform-canvas" class="w-full h-full opacity-60"></canvas>
            </div>
            <input id="expanded-timeline" type="range" min="0" max="100" value="0" class="w-full absolute inset-0 accent-accent bg-transparent h-1 cursor-pointer z-10">
          </div>
          <span id="expanded-time-remaining" class="min-w-[35px] sm:min-w-[40px] text-right">0:00</span>
        </div>
      </div>

      <!-- Playback buttons -->
      <div class="flex items-center justify-center space-x-4 sm:space-x-6">
        <button id="expanded-prev-chapter" class="p-1.5 sm:p-2 hover:bg-black-500 rounded text-black-50 hover:text-white transition-all ${currentItem.media?.chapters?.length > 0 ? '' : 'hidden'}" title="Previous Chapter">
          <span class="material-symbols text-lg sm:text-xl">first_page</span>
        </button>
        <button id="expanded-seek-back" class="p-1.5 sm:p-2 hover:bg-black-500 rounded text-black-50 hover:text-white transition-all" title="Seek Back">
          <span class="material-symbols text-lg sm:text-xl">replay_10</span>
        </button>
        
        <button id="expanded-play-pause" class="p-3 sm:p-3.5 bg-accent hover:opacity-90 rounded-full text-primary flex items-center justify-center animate-none shadow-lg hover:scale-105 transition-all" title="Play/Pause">
          <span id="expanded-play-pause-icon" class="material-symbols text-xl sm:text-2xl font-bold">play_arrow</span>
        </button>
        
        <button id="expanded-seek-forward" class="p-1.5 sm:p-2 hover:bg-black-500 rounded text-black-50 hover:text-white transition-all" title="Seek Forward">
          <span class="material-symbols text-lg sm:text-xl">forward_10</span>
        </button>
        <button id="expanded-next" class="p-1.5 sm:p-2 hover:bg-black-500 rounded text-black-50 hover:text-white transition-all ${currentItem.media?.chapters?.length > 0 || getQueueLength() > 0 ? '' : 'hidden'}" title="Next">
          <span class="material-symbols text-lg sm:text-xl">last_page</span>
        </button>
      </div>

      <!-- Secondary Controls -->
      <div class="border-t border-black-600/30 pt-3 sm:pt-4 flex flex-col space-y-2.5 sm:space-y-4">
        <div class="flex items-center justify-center space-x-2 sm:space-x-3 w-full max-w-xs sm:max-w-sm mx-auto px-4">
          <button id="expanded-volume-btn" class="p-1 hover:bg-black-500 rounded text-black-50 hover:text-white transition-all">
            <span id="expanded-volume-icon" class="material-symbols text-base sm:text-lg">volume_up</span>
          </button>
          <input id="expanded-volume-slider" type="range" min="0" max="100" value="${audio ? Math.round(audio.volume * 100) : 100}" class="flex-grow accent-accent bg-black-500 h-1 sm:h-1.5 rounded-lg cursor-pointer">
        </div>

        <div class="flex items-center justify-around text-black-50 px-1 sm:px-2 max-w-sm sm:max-w-md mx-auto w-full">
          <div class="flex flex-col items-center space-y-0.5">
            <select id="expanded-speed" class="bg-black-500 border border-black-300 rounded text-[10px] text-white px-1.5 py-0.5 focus:outline-none cursor-pointer">
              ${speedOptions}
            </select>
            <span class="text-[9px] text-black-100 font-semibold uppercase">Speed</span>
          </div>

          <button id="expanded-sleep-btn" class="p-1.5 sm:p-2 hover:bg-black-500 rounded hover:text-white flex flex-col items-center space-y-0.5 relative" title="Sleep Timer">
            <span id="expanded-sleep-icon" class="material-symbols text-base sm:text-lg">bedtime</span>
            <span id="expanded-sleep-badge" class="absolute top-1 right-1 sm:right-2 bg-accent text-primary text-[7px] sm:text-[8px] font-bold rounded-full w-3 h-3 sm:w-3.5 sm:h-3.5 flex items-center justify-center hidden"></span>
            <span class="text-[9px] text-black-100 font-semibold uppercase">Sleep</span>
          </button>

          <button id="expanded-chapters-btn" class="p-1.5 sm:p-2 hover:bg-black-500 rounded hover:text-white flex flex-col items-center space-y-0.5 ${currentItem.media?.chapters?.length > 0 ? '' : 'opacity-40 pointer-events-none'}" title="Chapters">
            <span class="material-symbols text-base sm:text-lg">format_list_bulleted</span>
            <span class="text-[9px] text-black-100 font-semibold uppercase">Chapters</span>
          </button>

          <button id="expanded-queue-btn" class="p-1.5 sm:p-2 hover:bg-black-500 rounded hover:text-white flex flex-col items-center space-y-0.5 relative" title="Queue">
            <span class="material-symbols text-base sm:text-lg">queue_music</span>
            <span id="expanded-queue-badge" class="absolute top-1 right-1 sm:right-2 bg-accent text-primary text-[7px] sm:text-[8px] font-bold rounded-full w-3 h-3 sm:w-3.5 sm:h-3.5 flex items-center justify-center hidden"></span>
            <span class="text-[9px] text-black-100 font-semibold uppercase">Queue</span>
          </button>

          <button id="expanded-bookmark-btn" class="p-1.5 sm:p-2 hover:bg-black-500 rounded hover:text-white flex flex-col items-center space-y-0.5" title="Add Bookmark">
            <span class="material-symbols text-base sm:text-lg">bookmark_add</span>
            <span class="text-[9px] text-black-100 font-semibold uppercase">Bookmark</span>
          </button>
        </div>
      </div>
    </div>
  `;

  document.body.appendChild(dialog);

  const closeExpandedPlayer = () => {
    dialog.close();
    dialog.remove();
  };
  dialog.querySelector('#expanded-close-btn').onclick = closeExpandedPlayer;

  dialog.showModal();

  setupUIEventListeners();

  const isPlaying = audio ? !audio.paused : false;
  updatePlayPauseButton(isPlaying);
  updateTimelineUI();
  updateSleepTimerUI();
  updateQueueUI();
  updateSkipButtonsUI();
}

export function setupUIEventListeners() {
  if (!playerController) return;
  const skipBack = playerController.getPlayerSkipBackSeconds();
  const skipForward = playerController.getPlayerSkipForwardSeconds();
  const currentItem = playerController.getCurrentItem();

  const bindClick = (ids, handler) => {
    ids.forEach(id => {
      const el = document.getElementById(id);
      if (el) el.onclick = handler;
    });
  };

  const handlePlayPause = () => {
    if (isCasting()) {
      getRemotePlayer().volumeLevel = playerController.getAudio() ? playerController.getAudio().volume : 1.0;
      // Socket/GCast trigger
      const session = cast.framework.CastContext.getInstance().getCurrentSession();
      if (session) {
        const controller = new cast.framework.RemotePlayerController(getRemotePlayer());
        controller.playOrPause();
      }
    } else {
      const audio = playerController.getAudio();
      if (!audio) return;
      if (audio.paused) {
        audio.play().catch(err => console.error('Play failed:', err));
      } else {
        audio.pause();
      }
    }
  };
  bindClick(['player-play-pause', 'player-play-pause-mobile', 'expanded-play-pause'], handlePlayPause);

  const handleSeekBack = () => {
    if (isCasting()) {
      const newTime = Math.max(getRemotePlayer().currentTime - skipBack, 0);
      getRemotePlayer().currentTime = newTime;
      const controller = new cast.framework.RemotePlayerController(getRemotePlayer());
      controller.seek();
    } else {
      const audio = playerController.getAudio();
      if (!audio) return;
      audio.currentTime = Math.max(audio.currentTime - skipBack, 0);
    }
  };
  bindClick(['player-seek-back', 'expanded-seek-back'], handleSeekBack);

  const handleSeekForward = () => {
    if (isCasting()) {
      const newTime = Math.min(getRemotePlayer().currentTime + skipForward, getRemotePlayer().duration || 0);
      getRemotePlayer().currentTime = newTime;
      const controller = new cast.framework.RemotePlayerController(getRemotePlayer());
      controller.seek();
    } else {
      const audio = playerController.getAudio();
      if (!audio) return;
      audio.currentTime = Math.min(audio.currentTime + skipForward, audio.duration || 0);
    }
  };
  bindClick(['player-seek-forward', 'expanded-seek-forward'], handleSeekForward);

  const handlePrevChapter = () => {
    const currentSecs = isCasting() ? getRemotePlayer().currentTime : (playerController.getAudio() ? playerController.getAudio().currentTime : 0);
    const chapters = currentItem?.media?.chapters || [];
    const activeChapter = chapters.find(c => currentSecs >= c.start && currentSecs < c.end);
    const activeChapterIndex = chapters.indexOf(activeChapter);
    
    if (activeChapterIndex === -1 || activeChapterIndex === 0) {
      playerController.seekTo(0);
    } else {
      const timeInCurrentChapter = currentSecs - activeChapter.start;
      if (timeInCurrentChapter <= 3 && chapters[activeChapterIndex - 1]) {
        playerController.seekTo(chapters[activeChapterIndex - 1].start);
      } else {
        playerController.seekTo(activeChapter.start);
      }
    }
  };
  bindClick(['player-prev-chapter', 'expanded-prev-chapter'], handlePrevChapter);

  const handleNext = () => {
    const chapters = currentItem?.media?.chapters || [];
    const currentSecs = isCasting() ? getRemotePlayer().currentTime : (playerController.getAudio() ? playerController.getAudio().currentTime : 0);
    const activeChapter = chapters.find(c => currentSecs >= c.start && currentSecs < c.end);
    const activeChapterIndex = chapters.indexOf(activeChapter);
    
    if (activeChapterIndex !== -1 && activeChapterIndex < chapters.length - 1) {
      const nextChapter = chapters[activeChapterIndex + 1];
      playerController.seekTo(nextChapter.start);
    } else if (getQueueLength() > 0) {
      playerController.playNextInQueue();
    }
  };
  bindClick(['player-next', 'expanded-next'], handleNext);

  const handleTimelineInput = (el) => {
    const val = parseFloat(el.value);
    if (isCasting()) {
      if (!getRemotePlayer().duration) return;
      const pct = val / 100;
      getRemotePlayer().currentTime = pct * getRemotePlayer().duration;
      const controller = new cast.framework.RemotePlayerController(getRemotePlayer());
      controller.seek();
    } else {
      const audio = playerController.getAudio();
      if (!audio || !audio.duration) return;
      const pct = val / 100;
      audio.currentTime = pct * audio.duration;
    }
    const otherId = el.id === 'player-timeline' ? 'expanded-timeline' : 'player-timeline';
    const other = document.getElementById(otherId);
    if (other) other.value = el.value;
    drawWaveform();
  };

  const setupTimeline = (id) => {
    const timeline = document.getElementById(id);
    if (!timeline) return;
    timeline.oninput = () => handleTimelineInput(timeline);

    if (timeline.dataset.hasHoverListeners === 'true') return;
    timeline.dataset.hasHoverListeners = 'true';

    let tooltip = document.getElementById(id + '-tooltip');
    if (!tooltip && timeline.parentElement) {
      tooltip = document.createElement('div');
      tooltip.id = id + '-tooltip';
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
      
      playerController.setHoverPct(pct);
      playerController.setHoverX(x);

      const duration = isCasting() ? (getRemotePlayer().duration || 0) : (playerController.getAudio() ? playerController.getAudio().duration : 0);
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
      playerController.setHoverPct(null);
      playerController.setHoverX(null);
      if (tooltip) tooltip.classList.add('hidden');
      drawWaveform();
    });

    const handleTouch = (e) => {
      if (!e.touches || e.touches.length === 0) return;
      const rect = timeline.getBoundingClientRect();
      const clientX = e.touches[0].clientX;
      const x = clientX - rect.left;
      const pct = Math.max(0, Math.min(1, x / rect.width));
      
      playerController.setHoverPct(pct);
      playerController.setHoverX(x);

      const duration = isCasting() ? (getRemotePlayer().duration || 0) : (playerController.getAudio() ? playerController.getAudio().duration : 0);
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
    };

    timeline.addEventListener('touchstart', (e) => {
      if (tooltip) tooltip.classList.remove('hidden');
      handleTouch(e);
    }, { passive: true });

    timeline.addEventListener('touchmove', (e) => {
      handleTouch(e);
    }, { passive: true });

    timeline.addEventListener('touchend', () => {
      playerController.setHoverPct(null);
      playerController.setHoverX(null);
      if (tooltip) tooltip.classList.add('hidden');
      drawWaveform();
    });
  };

  setupTimeline('player-timeline');
  setupTimeline('expanded-timeline');

  const syncVolumeSliders = (val) => {
    ['player-volume-slider', 'expanded-volume-slider'].forEach(id => {
      const el = document.getElementById(id);
      if (el) el.value = val;
    });
  };

  const handleVolumeMute = () => {
    let isMuted = !playerController.getIsMuted();
    playerController.setIsMuted(isMuted);

    if (isCasting()) {
      if (isMuted) {
        playerController.setPreviousVolume(getRemotePlayer().volumeLevel);
        getRemotePlayer().volumeLevel = 0;
        const controller = new cast.framework.RemotePlayerController(getRemotePlayer());
        controller.setVolumeLevel();
        updateVolumeIcon(0);
        syncVolumeSliders(0);
      } else {
        const prevVol = playerController.getPreviousVolume();
        getRemotePlayer().volumeLevel = prevVol;
        const controller = new cast.framework.RemotePlayerController(getRemotePlayer());
        controller.setVolumeLevel();
        updateVolumeIcon(prevVol);
        syncVolumeSliders(Math.round(prevVol * 100));
      }
    } else {
      const audio = playerController.getAudio();
      if (!audio) return;
      if (isMuted) {
        playerController.setPreviousVolume(playerController.getUserVolume());
        playerController.setUserVolume(0);
        if (!playerController.isFading()) audio.volume = 0;
        updateVolumeIcon(0);
        syncVolumeSliders(0);
      } else {
        const prevVol = playerController.getPreviousVolume();
        playerController.setUserVolume(prevVol);
        if (!playerController.isFading()) audio.volume = prevVol;
        updateVolumeIcon(prevVol);
        syncVolumeSliders(Math.round(prevVol * 100));
      }
    }
  };
  bindClick(['player-volume-btn', 'expanded-volume-btn'], handleVolumeMute);

  const handleVolumeSliderInput = (el) => {
    const val = parseFloat(el.value) / 100;
    playerController.setIsMuted(val === 0);
    updateVolumeIcon(val);
    syncVolumeSliders(el.value);
    if (isCasting()) {
      getRemotePlayer().volumeLevel = val;
      const controller = new cast.framework.RemotePlayerController(getRemotePlayer());
      controller.setVolumeLevel();
    } else {
      const audio = playerController.getAudio();
      if (!audio) return;
      playerController.setUserVolume(val);
      if (!playerController.isFading()) audio.volume = val;
    }
  };
  ['player-volume-slider', 'expanded-volume-slider'].forEach(id => {
    const el = document.getElementById(id);
    if (el) el.oninput = () => handleVolumeSliderInput(el);
  });

  const handleSpeedChange = (el) => {
    const audio = playerController.getAudio();
    if (!audio) return;
    const speedVal = parseFloat(el.value) || 1.0;
    audio.playbackRate = speedVal;
    
    const otherId = el.id === 'player-speed' ? 'expanded-speed' : 'player-speed';
    const other = document.getElementById(otherId);
    if (other) other.value = el.value;

    if (playerController.getRememberSpeedPerBook() && currentItem) {
      localStorage.setItem(`abs-speed-book-${currentItem.id}`, speedVal.toString());
    } else {
      playerController.setGlobalDefaultSpeed(speedVal);
      localStorage.setItem('abs-speed-global', speedVal.toString());
    }
  };
  ['player-speed', 'expanded-speed'].forEach(id => {
    const el = document.getElementById(id);
    if (el) el.onchange = () => handleSpeedChange(el);
  });

  bindClick(['player-close'], () => {
    playerController.destroyPlayer();
  });

  bindClick(['player-bookmark-btn', 'expanded-bookmark-btn'], triggerAddBookmarkModal);
  bindClick(['player-sleep-btn', 'expanded-sleep-btn'], triggerSleepTimerModal);
  bindClick(['player-settings-btn', 'expanded-settings-btn'], triggerPlayerSettingsModal);
  bindClick(['player-chapters-btn', 'expanded-chapters-btn'], triggerChaptersModal);
  bindClick(['player-queue-btn', 'expanded-queue-btn'], triggerQueueModal);

  const metaContainer = document.getElementById('player-meta-container');
  if (metaContainer) {
    metaContainer.onclick = () => {
      triggerExpandedPlayer();
    };
  }

  if (!window.hasPlayerKeyboardListeners) {
    window.hasPlayerKeyboardListeners = true;
    document.addEventListener('keydown', (e) => {
      if (!playerController || !playerController.getCurrentItem()) return;
      
      const activeEl = document.activeElement;
      if (activeEl) {
        const tagName = activeEl.tagName;
        if (tagName === 'INPUT' || tagName === 'TEXTAREA' || tagName === 'SELECT' || activeEl.isContentEditable) {
          return;
        }
      }
      
      if (e.code === 'Space') {
        e.preventDefault();
        handlePlayPause();
      } else if (e.code === 'ArrowLeft') {
        e.preventDefault();
        handleSeekBack();
      } else if (e.code === 'ArrowRight') {
        e.preventDefault();
        handleSeekForward();
      }
    });
  }

  if ('mediaSession' in navigator) {
    try {
      navigator.mediaSession.setActionHandler('play', handlePlayPause);
      navigator.mediaSession.setActionHandler('pause', handlePlayPause);
      navigator.mediaSession.setActionHandler('seekbackward', handleSeekBack);
      navigator.mediaSession.setActionHandler('seekforward', handleSeekForward);
      navigator.mediaSession.setActionHandler('previoustrack', handlePrevChapter);
      navigator.mediaSession.setActionHandler('nexttrack', handleNext);
      navigator.mediaSession.setActionHandler('seekto', (details) => {
        if (details.seekTime !== undefined) {
          playerController.seekTo(details.seekTime);
        }
      });
    } catch (err) {
      console.warn('Failed to set MediaSession action handlers:', err);
    }
  }
}
