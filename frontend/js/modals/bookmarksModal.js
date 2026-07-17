import { request } from '../api.js';
import { escapeHtml, formatDuration, parseDuration } from '../itemDetails.js';
import { getCurrentPlayingItem, getCurrentPlaybackTime, playItem } from '../player.js';

let currentUser = null;

async function getBookmarksUser() {
  if (!currentUser) {
    currentUser = await request('GET', '/api/me');
  }
  return currentUser;
}


export async function renderBookmarks(item) {
  const container = document.getElementById('bookmarks-list-container');
  if (!container) return;

  container.innerHTML = `
    <div class="flex items-center justify-center p-4">
      <div class="animate-spin rounded-full h-6 w-6 border-b-2 border-accent"></div>
    </div>
  `;

  try {
    currentUser = await request('GET', '/api/me');
    const user = await getBookmarksUser();
  const bookmarks = (user.bookmarks || []).filter(b => b.libraryItemId === item.id);
    
    bookmarks.sort((a, b) => a.time - b.time);

    if (bookmarks.length === 0) {
      container.innerHTML = `
        <p class="text-xs text-black-100 italic py-2">No bookmarks saved for this audiobook.</p>
      `;
      return;
    }

    container.innerHTML = `
      <ul class="space-y-1 border border-black-400/50 rounded-md p-2 bg-primary/20 max-h-60 overflow-y-auto no-scroll">
        ${bookmarks.map((b, idx) => `
          <li class="flex items-start justify-between p-2 hover:bg-black-500/40 rounded transition-colors text-xs" data-time="${b.time}">
            <div class="flex items-start space-x-2 truncate flex-grow cursor-pointer bookmark-jump-btn mr-4">
              <span class="material-symbols text-sm mt-0.5" style="color: ${b.color || '#e5a93c'}">bookmark</span>
              <div class="truncate">
                <p class="font-medium text-white truncate">${escapeHtml(b.title)}</p>
                <div class="flex items-center space-x-2 text-[0.7rem] text-black-100 mt-0.5">
                  <span>${formatDuration(b.time)}</span>
                </div>
                ${b.note ? `<p class="text-[0.68rem] text-black-200 mt-1 italic whitespace-pre-wrap break-words border-l border-black-400 pl-1.5">${escapeHtml(b.note)}</p>` : ''}
              </div>
            </div>
            <div class="flex items-center space-x-2 flex-shrink-0 mt-0.5">
              <button class="bookmark-edit-btn text-black-100 hover:text-white p-1 rounded" title="Edit Bookmark" data-idx="${idx}">
                <span class="material-symbols text-sm">edit</span>
              </button>
              <button class="bookmark-delete-btn text-black-100 hover:text-accent p-1 rounded" title="Delete Bookmark" data-idx="${idx}">
                <span class="material-symbols text-sm">delete</span>
              </button>
            </div>
          </li>
        `).join('')}
      </ul>
    `;

    container.querySelectorAll('.bookmark-jump-btn').forEach((btn, idx) => {
      btn.onclick = () => {
        const b = bookmarks[idx];
        playItem(item, b.time);
      };
    });

    container.querySelectorAll('.bookmark-edit-btn').forEach(btn => {
      btn.onclick = (e) => {
        e.stopPropagation();
        const idx = parseInt(btn.getAttribute('data-idx'), 10);
        const b = bookmarks[idx];
        triggerEditBookmarkModal(item, b);
      };
    });

    container.querySelectorAll('.bookmark-delete-btn').forEach(btn => {
      btn.onclick = async (e) => {
        e.stopPropagation();
        const idx = parseInt(btn.getAttribute('data-idx'), 10);
        const b = bookmarks[idx];
        const confirmed = await window.showConfirm(
          'Delete Bookmark',
          `Are you sure you want to delete the bookmark "${b.title}"?`,
          'Delete',
          'Cancel'
        );
        if (confirmed) {
          try {
            await request('DELETE', `/api/me/item/${item.id}/bookmark/${b.time}`);
            renderBookmarks(item);
          } catch (err) {
            console.error('Failed to delete bookmark:', err);
            showToast('Failed to delete bookmark.', 'error');
          }
        }
      };
    });

  } catch (err) {
    console.error('Failed to render bookmarks:', err);
    container.innerHTML = `<p class="text-xs text-red-500">Failed to load bookmarks.</p>`;
  }
}

export function triggerEditBookmarkModal(item, bookmark) {
  const modal = document.createElement('div');
  modal.className = 'fixed inset-0 bg-black-900/80 backdrop-blur-sm z-50 flex items-center justify-center p-4 select-none';
  modal.innerHTML = `
    <div class="bg-primary border border-black-400 rounded-lg max-w-md w-full p-6 shadow-2xl space-y-4">
      <div class="flex items-center justify-between border-b border-black-500 pb-3">
        <h3 class="text-sm font-bold text-white uppercase tracking-wider flex items-center space-x-1.5">
          <span class="material-symbols text-base text-accent">edit</span>
          <span>Edit Bookmark</span>
        </h3>
        <button id="close-edit-bookmark-modal" class="text-black-100 hover:text-white transition-colors">
          <span class="material-symbols text-lg">close</span>
        </button>
      </div>

      <div class="space-y-3 text-left">
        <div>
          <label class="text-[0.65rem] uppercase font-bold text-black-100 tracking-wider">Bookmark Time</label>
          <p class="text-white font-semibold text-sm">${formatDuration(bookmark.time)}</p>
        </div>
        <div>
          <label for="edit-bookmark-title-input" class="text-[0.65rem] uppercase font-bold text-black-100 tracking-wider block mb-1">Bookmark Title</label>
          <input type="text" id="edit-bookmark-title-input" required class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent text-xs">
        </div>
        <div>
          <label for="edit-bookmark-note-input" class="text-[0.65rem] uppercase font-bold text-black-100 tracking-wider block mb-1">Notes</label>
          <textarea id="edit-bookmark-note-input" rows="2" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent text-xs placeholder-black-200" placeholder="Optional notes..."></textarea>
        </div>
        <div>
          <label class="text-[0.65rem] uppercase font-bold text-black-100 tracking-wider block mb-1.5">Color Tag</label>
          <div class="flex items-center space-x-2" id="edit-bookmark-color-options">
            <button type="button" class="w-6 h-6 rounded-full border-2 ${bookmark.color === '#e5a93c' || !bookmark.color ? 'border-accent' : 'border-transparent'} transition-all color-option-btn" data-color="#e5a93c" style="background-color: #e5a93c;" title="Amber"></button>
            <button type="button" class="w-6 h-6 rounded-full border-2 ${bookmark.color === '#ef4444' ? 'border-accent' : 'border-transparent'} transition-all color-option-btn" data-color="#ef4444" style="background-color: #ef4444;" title="Red"></button>
            <button type="button" class="w-6 h-6 rounded-full border-2 ${bookmark.color === '#f97316' ? 'border-accent' : 'border-transparent'} transition-all color-option-btn" data-color="#f97316" style="background-color: #f97316;" title="Orange"></button>
            <button type="button" class="w-6 h-6 rounded-full border-2 ${bookmark.color === '#10b981' ? 'border-accent' : 'border-transparent'} transition-all color-option-btn" data-color="#10b981" style="background-color: #10b981;" title="Green"></button>
            <button type="button" class="w-6 h-6 rounded-full border-2 ${bookmark.color === '#3b82f6' ? 'border-accent' : 'border-transparent'} transition-all color-option-btn" data-color="#3b82f6" style="background-color: #3b82f6;" title="Blue"></button>
            <button type="button" class="w-6 h-6 rounded-full border-2 ${bookmark.color === '#8b5cf6' ? 'border-accent' : 'border-transparent'} transition-all color-option-btn" data-color="#8b5cf6" style="background-color: #8b5cf6;" title="Purple"></button>
          </div>
        </div>
      </div>

      <div class="flex items-center justify-end space-x-3 pt-3 border-t border-black-500">
        <button id="cancel-edit-bookmark-btn" class="bg-transparent hover:bg-black-500 text-white px-4 py-2 rounded text-xs transition-colors">
          Cancel
        </button>
        <button id="save-edit-bookmark-btn" class="bg-accent text-primary font-bold px-4 py-2 rounded text-xs hover:opacity-90 transition-opacity">
          Save Changes
        </button>
      </div>
    </div>
  `;

  document.body.appendChild(modal);

  const titleInput = document.getElementById('edit-bookmark-title-input');
  const noteInput = document.getElementById('edit-bookmark-note-input');
  
  titleInput.value = bookmark.title;
  noteInput.value = bookmark.note || '';

  let selectedColor = bookmark.color || '#e5a93c';
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

  titleInput.focus();
  titleInput.select();

  const closeModal = () => modal.remove();
  document.getElementById('close-edit-bookmark-modal').onclick = closeModal;
  document.getElementById('cancel-edit-bookmark-btn').onclick = closeModal;

  document.getElementById('save-edit-bookmark-btn').onclick = async () => {
    const titleVal = titleInput.value.trim();
    if (!titleVal) {
      showToast("Title is required", "warning");
      return;
    }

    try {
      await request('PATCH', `/api/me/item/${item.id}/bookmark`, {
        time: bookmark.time,
        title: titleVal,
        note: noteInput.value.trim(),
        color: selectedColor
      });
      closeModal();
      renderBookmarks(item);
    } catch (err) {
      console.error('Failed to update bookmark:', err);
      showToast('Failed to update bookmark: ' + (err.message || 'Unknown error'), 'error');
    }
  };
}

export function triggerAddBookmarkOnDetailsModal(item) {
  const playingItem = getCurrentPlayingItem();
  const isPlayingThis = playingItem && playingItem.id === item.id;
  const activeTime = isPlayingThis ? getCurrentPlaybackTime() : 0;

  let hrs = Math.floor(activeTime / 3600);
  let mins = Math.floor((activeTime % 3600) / 60);
  let secs = Math.floor(activeTime % 60);
  let timeStr = "";
  if (hrs > 0) {
    timeStr += `${hrs}:${mins < 10 ? '0' : ''}${mins}:${secs < 10 ? '0' : ''}${secs}`;
  } else {
    timeStr += `${mins}:${secs < 10 ? '0' : ''}${secs}`;
  }

  const modal = document.createElement('div');
  modal.className = 'fixed inset-0 bg-black-900/80 backdrop-blur-sm z-50 flex items-center justify-center p-4 select-none';
  modal.innerHTML = `
    <div class="bg-primary border border-black-400 rounded-lg max-w-md w-full p-6 shadow-2xl space-y-4">
      <div class="flex items-center justify-between border-b border-black-500 pb-3">
        <h3 class="text-sm font-bold text-white uppercase tracking-wider flex items-center space-x-1.5">
          <span class="material-symbols text-base text-accent">bookmark</span>
          <span>Add Bookmark</span>
        </h3>
        <button id="close-add-bookmark-modal" class="text-black-100 hover:text-white transition-colors">
          <span class="material-symbols text-lg">close</span>
        </button>
      </div>

      <div class="space-y-3 text-left">
        <div>
          <label for="add-bookmark-time-input" class="text-[0.65rem] uppercase font-bold text-black-100 tracking-wider block mb-1">Bookmark Time (HH:MM:SS or Seconds)</label>
          <input type="text" id="add-bookmark-time-input" required class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent text-xs" placeholder="e.g. 1:15:30 or 4530">
        </div>
        <div>
          <label for="add-bookmark-title-input" class="text-[0.65rem] uppercase font-bold text-black-100 tracking-wider block mb-1">Bookmark Title</label>
          <input type="text" id="add-bookmark-title-input" required class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent text-xs" placeholder="e.g. Favorite Quote">
        </div>
        <div>
          <label for="add-bookmark-note-input" class="text-[0.65rem] uppercase font-bold text-black-100 tracking-wider block mb-1">Notes</label>
          <textarea id="add-bookmark-note-input" rows="2" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent text-xs placeholder-black-200" placeholder="Optional notes..."></textarea>
        </div>
        <div>
          <label class="text-[0.65rem] uppercase font-bold text-black-100 tracking-wider block mb-1.5">Color Tag</label>
          <div class="flex items-center space-x-2" id="add-bookmark-color-options">
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
        <button id="cancel-add-bookmark-btn" class="bg-transparent hover:bg-black-500 text-white px-4 py-2 rounded text-xs transition-colors">
          Cancel
        </button>
        <button id="save-add-bookmark-btn" class="bg-accent text-primary font-bold px-4 py-2 rounded text-xs hover:opacity-90 transition-opacity">
          Save Bookmark
        </button>
      </div>
    </div>
  `;

  document.body.appendChild(modal);

  const timeInput = document.getElementById('add-bookmark-time-input');
  const titleInput = document.getElementById('add-bookmark-title-input');
  const noteInput = document.getElementById('add-bookmark-note-input');

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

  timeInput.value = timeStr;
  titleInput.value = `Bookmark at ${timeStr}`;

  timeInput.oninput = () => {
    const currentVal = timeInput.value.trim();
    if (currentVal) {
      titleInput.value = `Bookmark at ${currentVal}`;
    }
  };

  const closeModal = () => modal.remove();
  document.getElementById('close-add-bookmark-modal').onclick = closeModal;
  document.getElementById('cancel-add-bookmark-btn').onclick = closeModal;

  document.getElementById('save-add-bookmark-btn').onclick = async () => {
    const rawTime = timeInput.value.trim();
    const titleVal = titleInput.value.trim();

    if (!rawTime || !titleVal) {
      showToast("Both time and title are required.", "warning");
      return;
    }

    let parsedTime = 0;
    if (rawTime.includes(':')) {
      const parts = rawTime.split(':').map(Number);
      if (parts.some(isNaN)) {
        showToast("Invalid time format. Please use HH:MM:SS or simple seconds.", "warning");
        return;
      }
      if (parts.length === 3) {
        parsedTime = parts[0] * 3600 + parts[1] * 60 + parts[2];
      } else if (parts.length === 2) {
        parsedTime = parts[0] * 60 + parts[1];
      } else {
        showToast("Invalid time format. Please use HH:MM:SS or simple seconds.", "warning");
        return;
      }
    } else {
      parsedTime = Number(rawTime);
      if (isNaN(parsedTime)) {
        showToast("Invalid time format. Please use HH:MM:SS or simple seconds.", "warning");
        return;
      }
    }

    try {
      await request('POST', `/api/me/item/${item.id}/bookmark`, {
        time: parsedTime,
        title: titleVal,
        note: noteInput.value.trim(),
        color: selectedColor
      });
      closeModal();
      renderBookmarks(item);
    } catch (err) {
      console.error('Failed to create bookmark:', err);
      showToast('Failed to save bookmark: ' + (err.message || 'Unknown error'), 'error');
    }
  };
}

export async function triggerExportBookmarksModal(item) {
  const user = await getBookmarksUser();
  const bookmarks = (user.bookmarks || []).filter(b => b.libraryItemId === item.id);
  bookmarks.sort((a, b) => a.time - b.time);

  if (bookmarks.length === 0) {
    showToast("No bookmarks to export.", "warning");
    return;
  }

  const modal = document.createElement('div');
  modal.className = 'fixed inset-0 bg-black-900/80 backdrop-blur-sm z-50 flex items-center justify-center p-4 select-none';
  modal.innerHTML = `
    <div class="bg-primary border border-black-400 rounded-lg max-w-md w-full p-6 shadow-2xl space-y-4">
      <div class="flex items-center justify-between border-b border-black-500 pb-3">
        <h3 class="text-sm font-bold text-white uppercase tracking-wider flex items-center space-x-1.5">
          <span class="material-symbols text-base text-accent">download</span>
          <span>Export Bookmarks</span>
        </h3>
        <button id="close-export-bookmark-modal" class="text-black-100 hover:text-white transition-colors">
          <span class="material-symbols text-lg">close</span>
        </button>
      </div>

      <div class="space-y-3 text-center py-2">
        <p class="text-xs text-black-100">Select the file format to export your bookmarks for <span class="text-white font-semibold">"${escapeHtml(item.title)}"</span>.</p>
        
        <div class="flex flex-col space-y-2 pt-2">
          <button id="export-txt-btn" class="w-full bg-black-500 hover:bg-black-400 text-white text-xs py-2.5 px-4 rounded border border-black-300 font-semibold text-left flex items-center justify-between transition-colors">
            <span>Text Format (.txt)</span>
            <span class="text-[0.65rem] text-black-100 font-normal">[00:05:23] Chapter 1</span>
          </button>
          <button id="export-csv-btn" class="w-full bg-black-500 hover:bg-black-400 text-white text-xs py-2.5 px-4 rounded border border-black-300 font-semibold text-left flex items-center justify-between transition-colors">
            <span>CSV Table (.csv)</span>
            <span class="text-[0.65rem] text-black-100 font-normal">Time,Timestamp,Title</span>
          </button>
          <button id="export-json-btn" class="w-full bg-black-500 hover:bg-black-400 text-white text-xs py-2.5 px-4 rounded border border-black-300 font-semibold text-left flex items-center justify-between transition-colors">
            <span>JSON Payload (.json)</span>
            <span class="text-[0.65rem] text-black-100 font-normal">{"time": 323, ...}</span>
          </button>
        </div>
      </div>

      <div class="flex items-center justify-end pt-3 border-t border-black-500">
        <button id="cancel-export-bookmark-btn" class="bg-transparent hover:bg-black-500 text-white px-4 py-2 rounded text-xs transition-colors">
          Cancel
        </button>
      </div>
    </div>
  `;

  document.body.appendChild(modal);

  const closeModal = () => modal.remove();
  document.getElementById('close-export-bookmark-modal').onclick = closeModal;
  document.getElementById('cancel-export-bookmark-btn').onclick = closeModal;

  const downloadFile = (filename, content, mimeType) => {
    const blob = new Blob([content], { type: mimeType });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = filename;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);
    closeModal();
  };

  const sanitizeName = (name) => name.replace(/[\\/*?:"<>|]/g, "").trim() || "audiobook";

  document.getElementById('export-txt-btn').onclick = () => {
    let content = `Bookmarks for "${item.title}"\n`;
    content += `Generated on ${new Date().toLocaleString()}\n`;
    content += `-------------------------------------------\n\n`;
    bookmarks.forEach(b => {
      content += `[${formatDuration(b.time)}] ${b.title}\n`;
    });
    const filename = `${sanitizeName(item.title)}_bookmarks.txt`;
    downloadFile(filename, content, 'text/plain;charset=utf-8');
  };

  document.getElementById('export-csv-btn').onclick = () => {
    let content = `"Time (seconds)","Timestamp","Title","Note","Color"\n`;
    bookmarks.forEach(b => {
      content += `"${b.time}","${formatDuration(b.time)}","${(b.title || '').replace(/"/g, '""')}","${(b.note || '').replace(/"/g, '""')}","${(b.color || '#e5a93c').replace(/"/g, '""')}"\n`;
    });
    const filename = `${sanitizeName(item.title)}_bookmarks.csv`;
    downloadFile(filename, content, 'text/csv;charset=utf-8');
  };

  document.getElementById('export-json-btn').onclick = () => {
    const data = bookmarks.map(b => ({
      time: b.time,
      timestamp: formatDuration(b.time),
      title: b.title,
      note: b.note || '',
      color: b.color || '#e5a93c'
    }));
    const content = JSON.stringify(data, null, 2);
    const filename = `${sanitizeName(item.title)}_bookmarks.json`;
    downloadFile(filename, content, 'application/json;charset=utf-8');
  };
}

export function triggerImportBookmarksModal(item) {
  const modal = document.createElement('div');
  modal.className = 'fixed inset-0 bg-black-900/80 backdrop-blur-sm z-50 flex items-center justify-center p-4 select-none';
  modal.innerHTML = `
    <div class="bg-primary border border-black-400 rounded-lg max-w-md w-full p-6 shadow-2xl space-y-4">
      <div class="flex items-center justify-between border-b border-black-500 pb-3">
        <h3 class="text-sm font-bold text-white uppercase tracking-wider flex items-center space-x-1.5">
          <span class="material-symbols text-base text-accent">upload</span>
          <span>Import Bookmarks</span>
        </h3>
        <button id="close-import-bookmark-modal" class="text-black-100 hover:text-white transition-colors">
          <span class="material-symbols text-lg">close</span>
        </button>
      </div>
      
      <div class="space-y-4 text-left">
        <p class="text-xs text-black-100">Select a JSON or CSV file containing bookmarks to import for <span class="text-white font-semibold">"${escapeHtml(item.title)}"</span>.</p>
        
        <div class="flex items-center space-x-3">
          <button id="select-import-file-btn" class="bg-black-500 hover:bg-black-400 text-white border border-black-300 rounded px-3 py-2 text-xs font-semibold flex items-center space-x-1.5 transition-colors">
            <span class="material-symbols text-sm">attach_file</span>
            <span>Choose File...</span>
          </button>
          <span id="selected-file-name" class="text-xs text-black-200 truncate italic">No file selected</span>
          <input type="file" id="import-file-input" accept=".json,.csv" class="hidden">
        </div>

        <div id="import-preview-area" class="hidden border border-black-400/50 rounded-md p-3 bg-primary/20 space-y-2">
          <p class="text-xs font-bold text-white" id="import-preview-status"></p>
          <div class="max-h-32 overflow-y-auto no-scroll text-[0.7rem] text-black-100 space-y-1" id="import-preview-list"></div>
          
          <div class="flex items-center space-x-2 pt-2 border-t border-black-500/50">
            <input type="checkbox" id="import-overwrite-checkbox" class="rounded border-black-300 bg-black-500 text-accent focus:ring-accent w-3.5 h-3.5">
            <label for="import-overwrite-checkbox" class="text-[0.7rem] text-black-100 select-none">Overwrite existing bookmarks</label>
          </div>
        </div>
      </div>

      <div class="flex items-center justify-end space-x-3 pt-3 border-t border-black-500">
        <button id="cancel-import-bookmark-btn" class="bg-transparent hover:bg-black-500 text-white px-4 py-2 rounded text-xs transition-colors">
          Cancel
        </button>
        <button id="start-import-bookmark-btn" disabled class="bg-accent text-primary disabled:opacity-50 disabled:cursor-not-allowed font-bold px-4 py-2 rounded text-xs hover:opacity-90 transition-opacity">
          Import Bookmarks
        </button>
      </div>
    </div>
  `;

  document.body.appendChild(modal);

  const fileInput = document.getElementById('import-file-input');
  const selectBtn = document.getElementById('select-import-file-btn');
  const fileNameSpan = document.getElementById('selected-file-name');
  const previewArea = document.getElementById('import-preview-area');
  const previewStatus = document.getElementById('import-preview-status');
  const previewList = document.getElementById('import-preview-list');
  const startBtn = document.getElementById('start-import-bookmark-btn');
  const overwriteCheckbox = document.getElementById('import-overwrite-checkbox');

  let parsedBookmarks = [];

  const closeModal = () => modal.remove();
  document.getElementById('close-import-bookmark-modal').onclick = closeModal;
  document.getElementById('cancel-import-bookmark-btn').onclick = closeModal;

  selectBtn.onclick = () => fileInput.click();

  function parseJSONBookmarks(content) {
    const data = JSON.parse(content);
    const items = Array.isArray(data) ? data : [data];
    const list = [];
    for (const raw of items) {
      let time = 0;
      if (typeof raw.time === 'number') {
        time = raw.time;
      } else if (typeof raw.time === 'string') {
        time = parseDuration(raw.time);
      } else if (typeof raw.timestamp === 'string') {
        time = parseDuration(raw.timestamp);
      } else {
        continue;
      }
      const title = raw.title || `Bookmark at ${formatDuration(time)}`;
      const note = raw.note || '';
      const color = raw.color || '#e5a93c';
      list.push({ time, title, note, color });
    }
    return list;
  }

  function parseCSVBookmarks(content) {
    const lines = content.split(/\r?\n/).filter(line => line.trim().length > 0);
    if (lines.length < 2) {
      throw new Error("CSV file is empty or missing data lines.");
    }
    
    function parseCSVLine(line) {
      const result = [];
      let current = '';
      let inQuotes = false;
      for (let i = 0; i < line.length; i++) {
        const char = line[i];
        if (char === '"') {
          if (inQuotes && line[i + 1] === '"') {
            current += '"';
            i++;
          } else {
            inQuotes = !inQuotes;
          }
        } else if (char === ',' && !inQuotes) {
          result.push(current);
          current = '';
        } else {
          current += char;
        }
      }
      result.push(current);
      return result;
    }

    const headers = parseCSVLine(lines[0]).map(h => h.trim().toLowerCase());
    const timeIdx = headers.findIndex(h => h.includes('time'));
    const titleIdx = headers.findIndex(h => h.includes('title'));
    const noteIdx = headers.findIndex(h => h.includes('note'));
    const colorIdx = headers.findIndex(h => h.includes('color'));
    const timestampIdx = headers.findIndex(h => h.includes('timestamp'));

    if (timeIdx === -1 && timestampIdx === -1) {
      throw new Error("CSV must contain a 'Time' or 'Timestamp' column.");
    }

    const list = [];
    for (let i = 1; i < lines.length; i++) {
      const cols = parseCSVLine(lines[i]);
      if (cols.length < Math.max(timeIdx, timestampIdx, titleIdx) + 1) continue;

      let time = 0;
      if (timeIdx !== -1) {
        time = parseFloat(cols[timeIdx]) || 0;
      } else if (timestampIdx !== -1) {
        time = parseDuration(cols[timestampIdx]);
      }

      const title = (titleIdx !== -1 && cols[titleIdx]) ? cols[titleIdx].trim() : `Bookmark at ${formatDuration(time)}`;
      const note = (noteIdx !== -1 && cols[noteIdx]) ? cols[noteIdx].trim() : '';
      const color = (colorIdx !== -1 && cols[colorIdx]) ? cols[colorIdx].trim() : '#e5a93c';

      list.push({ time, title, note, color });
    }
    return list;
  }

  fileInput.onchange = (e) => {
    const file = e.target.files[0];
    if (!file) return;

    fileNameSpan.textContent = file.name;
    const reader = new FileReader();

    reader.onload = (event) => {
      const content = event.target.result;
      try {
        if (file.name.endsWith('.json')) {
          parsedBookmarks = parseJSONBookmarks(content);
        } else if (file.name.endsWith('.csv')) {
          parsedBookmarks = parseCSVBookmarks(content);
        } else {
          throw new Error("Unsupported file extension. Please upload a .json or .csv file.");
        }

        if (parsedBookmarks.length === 0) {
          throw new Error("No valid bookmarks found in the file.");
        }

        previewStatus.textContent = `Found ${parsedBookmarks.length} bookmark(s) ready to import:`;
        previewList.innerHTML = parsedBookmarks.map(b => `
          <div class="flex items-center justify-between border-b border-black-500/20 py-1">
            <span class="truncate pr-2 font-medium text-white">${escapeHtml(b.title)}</span>
            <span class="text-accent flex-shrink-0">${formatDuration(b.time)}</span>
          </div>
        `).join('');

        previewArea.classList.remove('hidden');
        startBtn.disabled = false;
      } catch (err) {
        showToast("Error parsing file: " + err.message, "error");
        fileNameSpan.textContent = "Error parsing file";
        previewArea.classList.add('hidden');
        startBtn.disabled = true;
        parsedBookmarks = [];
      }
    };

    reader.readAsText(file);
  };

  startBtn.onclick = async () => {
    startBtn.disabled = true;
    startBtn.textContent = "Importing...";
    
    try {
      const user = await getBookmarksUser();
      const existing = (user.bookmarks || []).filter(b => b.libraryItemId === item.id);
      
      if (overwriteCheckbox.checked) {
        for (const eb of existing) {
          try {
            await request('DELETE', `/api/me/item/${item.id}/bookmark/${eb.time}`);
          } catch (err) {
            console.warn(`Failed to delete bookmark at ${eb.time}:`, err);
          }
        }
      }

      const activeExisting = overwriteCheckbox.checked ? [] : existing;

      for (const pb of parsedBookmarks) {
        if (activeExisting.some(eb => Math.abs(eb.time - pb.time) < 0.1)) {
          continue;
        }

        try {
          await request('POST', `/api/me/item/${item.id}/bookmark`, {
            time: pb.time,
            title: pb.title,
            note: pb.note,
            color: pb.color
          });
        } catch (err) {
          console.error(`Failed to import bookmark at ${pb.time}:`, err);
        }
      }

      closeModal();
      renderBookmarks(item);
    } catch (err) {
      console.error('Import failed:', err);
      showToast('Import failed: ' + (err.message || 'Unknown error'), 'error');
      startBtn.disabled = false;
      startBtn.textContent = "Import Bookmarks";
    }
  };
}


/**
 * Triggers interactive chapter editor modal
 */
