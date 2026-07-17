import { resolvePath } from '../api.js';

let playbackQueue = [];
let draggedQueueIndex = null;
let notificationCallback = () => {};

export function registerNotificationCallback(cb) {
  notificationCallback = cb;
}

function escapeHtml(str) {
  if (!str) return '';
  return str
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#039;');
}

function formatTime(secs) {
  if (isNaN(secs) || secs === Infinity || secs === null || secs === undefined) return '0:00';
  const hours = Math.floor(secs / 3600);
  const minutes = Math.floor((secs % 3600) / 60);
  const seconds = Math.floor(secs % 60);
  
  const formattedSeconds = seconds < 10 ? `0${seconds}` : seconds;
  if (hours > 0) {
    const formattedMinutes = minutes < 10 ? `0${formattedMinutes}` : minutes;
    return `${hours}:${formattedMinutes}:${formattedSeconds}`;
  }
  return `${minutes}:${formattedSeconds}`;
}

export function getQueue() {
  return playbackQueue;
}

export function getQueueLength() {
  return playbackQueue.length;
}

export function setQueue(newQueue) {
  playbackQueue = newQueue;
  updateQueueUI();
}

export function queueShift() {
  const item = playbackQueue.shift();
  updateQueueUI();
  return item;
}

export function queueSome(callback) {
  return playbackQueue.some(callback);
}

export function addToQueue(itemObj) {
  if (playbackQueue.some(q => q.id === itemObj.id)) {
    notificationCallback('Item is already in queue');
    return;
  }
  playbackQueue.push(itemObj);
  updateQueueUI();
  const title = itemObj.media?.metadata?.title || itemObj.title || 'Item';
  notificationCallback(`Added "${title}" to queue`);
}

export function clearQueue() {
  playbackQueue = [];
  updateQueueUI();
  notificationCallback('Queue cleared');
}

export function removeFromQueue(index) {
  if (index >= 0 && index < playbackQueue.length) {
    const removed = playbackQueue.splice(index, 1)[0];
    updateQueueUI();
    const title = removed.media?.metadata?.title || removed.title || 'Item';
    notificationCallback(`Removed "${title}" from queue`);
  }
}

export function reorderQueue(fromIndex, toIndex) {
  if (fromIndex >= 0 && fromIndex < playbackQueue.length && toIndex >= 0 && toIndex < playbackQueue.length) {
    const element = playbackQueue.splice(fromIndex, 1)[0];
    playbackQueue.splice(toIndex, 0, element);
    updateQueueUI();
  }
}

export function updateQueueUI() {
  ['player-queue-badge', 'expanded-queue-badge'].forEach(id => {
    const badge = document.getElementById(id);
    if (badge) {
      if (playbackQueue.length > 0) {
        badge.textContent = playbackQueue.length;
        badge.classList.remove('hidden');
      } else {
        badge.classList.add('hidden');
      }
    }
  });

  // If queue modal is open, re-render its content
  const dialog = document.getElementById('player-queue-dialog');
  if (dialog && dialog.open) {
    renderQueueDialogContent(dialog);
  }

  // Fire event to let expanded UI or mini player know queue changed
  window.dispatchEvent(new CustomEvent('player-queue-changed'));
}

export function renderQueueDialogContent(dialog) {
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

    // Attach drag and drop events
    row.addEventListener('mousedown', (e) => {
      if (e.target.closest('.drag-handle')) {
        row.setAttribute('draggable', 'true');
      } else {
        row.setAttribute('draggable', 'false');
      }
    });

    row.addEventListener('dragstart', (e) => {
      if (row.getAttribute('draggable') !== 'true') {
        e.preventDefault();
        return;
      }
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

export function triggerQueueModal() {
  let dialog = document.getElementById('player-queue-dialog');
  if (dialog) {
    dialog.remove();
  }

  dialog = document.createElement('dialog');
  dialog.id = 'player-queue-dialog';
  dialog.setAttribute('closedby', 'any');
  dialog.className = 'bg-primary border border-black-400 rounded-lg max-w-md w-full p-6 shadow-2xl space-y-4 focus:outline-none text-white backdrop:bg-black-900/80 backdrop:backdrop-blur-sm open:flex open:flex-col open:items-stretch';

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
      <button id="clear-queue-btn" class="bg-red-900/40 hover:bg-red-900/60 border border-red-500/30 text-error hover:text-white hover:border-red-500/50 px-3 py-1.5 rounded text-xs transition-colors flex items-center space-x-1 cursor-pointer">
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
