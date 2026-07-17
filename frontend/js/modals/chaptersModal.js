import { request } from '../api.js';
import { formatDuration, escapeHtml, parseDuration } from '../itemDetails.js';

export function triggerEditChaptersModal(item, onSaveSuccess) {
  const modal = document.createElement('div');
  modal.className = 'fixed inset-0 bg-black-900/80 backdrop-blur-sm z-50 flex items-center justify-center p-4 select-none';

  let currentChapters = JSON.parse(JSON.stringify(item.media?.chapters || []));
  if (!Array.isArray(currentChapters)) {
    currentChapters = [];
  }

  const renderChaptersList = () => {
    const listContainer = modal.querySelector('#chapters-editor-list');
    if (!listContainer) return;

    if (currentChapters.length === 0) {
      listContainer.innerHTML = `
        <div class="text-center py-8 text-black-100 text-xs">
          No chapters. Click "Add Chapter" or "Audnexus Lookup" to populate chapters.
        </div>
      `;
      return;
    }

    listContainer.innerHTML = currentChapters.map((chap, idx) => `
      <div class="flex items-center space-x-2 bg-black-500/40 p-2 rounded border border-black-400/50 text-xs chapter-row" data-idx="${idx}">
        <span class="text-black-100 font-semibold w-6 text-center">${idx + 1}</span>
        
        <div class="flex-grow min-w-0">
          <input type="text" class="w-full bg-black-500 text-white px-2 py-1 rounded border border-black-300 focus:outline-none focus:border-accent text-xs chapter-title" value="${escapeHtml(chap.title)}" placeholder="Chapter Title">
        </div>
        
        <div class="w-24">
          <input type="text" class="w-full bg-black-500 text-white px-2 py-1 rounded border border-black-300 focus:outline-none focus:border-accent text-xs text-center chapter-start" value="${formatDuration(chap.start)}" placeholder="Start (HH:MM:SS)">
        </div>

        <div class="w-24">
          <input type="text" class="w-full bg-black-500 text-white px-2 py-1 rounded border border-black-300 focus:outline-none focus:border-accent text-xs text-center chapter-end" value="${formatDuration(chap.end)}" placeholder="End (HH:MM:SS)">
        </div>

        <button class="text-red-500 hover:text-red-400 transition-colors p-1 delete-chapter-btn">
          <span class="material-symbols text-sm">delete</span>
        </button>
      </div>
    `).join('');

    const rows = listContainer.querySelectorAll('.chapter-row');
    rows.forEach(row => {
      const idx = parseInt(row.getAttribute('data-idx'), 10);
      
      const titleInput = row.querySelector('.chapter-title');
      titleInput.oninput = (e) => {
        currentChapters[idx].title = e.target.value;
      };

      const startInput = row.querySelector('.chapter-start');
      startInput.onchange = (e) => {
        currentChapters[idx].start = parseDuration(e.target.value);
        e.target.value = formatDuration(currentChapters[idx].start);
      };

      const endInput = row.querySelector('.chapter-end');
      endInput.onchange = (e) => {
        currentChapters[idx].end = parseDuration(e.target.value);
        e.target.value = formatDuration(currentChapters[idx].end);
      };

      const deleteBtn = row.querySelector('.delete-chapter-btn');
      deleteBtn.onclick = () => {
        currentChapters.splice(idx, 1);
        renderChaptersList();
      };
    });
  };

  modal.innerHTML = `
    <div class="bg-primary border border-black-400 rounded-lg max-w-2xl w-full p-6 shadow-2xl space-y-4 flex flex-col max-h-[85vh]">
      <div class="flex items-center justify-between border-b border-black-500 pb-3 flex-shrink-0">
        <h3 class="text-sm font-bold text-white uppercase tracking-wider flex items-center space-x-1.5">
          <span class="material-symbols text-base text-accent">toc</span>
          <span>Edit Book Chapters</span>
        </h3>
        <button id="close-edit-chapters-modal" class="text-black-100 hover:text-white transition-colors">
          <span class="material-symbols text-lg">close</span>
        </button>
      </div>

      <div class="flex items-center justify-between bg-black-600/30 p-2.5 rounded border border-black-500/50 flex-shrink-0 text-xs">
        <div class="flex items-center space-x-2">
          <button id="editor-add-chapter-btn" class="bg-black-500 hover:bg-black-400 border border-black-300 text-white font-semibold px-2.5 py-1.5 rounded transition-colors flex items-center space-x-1">
            <span class="material-symbols text-sm">add</span>
            <span>Add Chapter</span>
          </button>
          <button id="editor-lookup-btn" class="bg-black-500 hover:bg-black-400 border border-black-300 text-accent font-semibold px-2.5 py-1.5 rounded transition-colors flex items-center space-x-1">
            <span class="material-symbols text-sm">search</span>
            <span>Audnexus Lookup</span>
          </button>
        </div>
        <div class="text-black-100 text-[0.7rem]">
          ASIN: <span class="text-white font-semibold">${escapeHtml(item.media?.metadata?.asin || 'None')}</span>
        </div>
      </div>

      <div id="chapters-editor-list" class="space-y-2 overflow-y-auto no-scroll flex-grow pr-1 min-h-[200px]">
        <!-- Dynamic chapters -->
      </div>

      <div class="flex items-center justify-between pt-3 border-t border-black-500 flex-shrink-0">
        <div class="text-[0.65rem] text-black-100 flex items-center space-x-1">
          <span class="material-symbols text-xs">info</span>
          <span>Times can be entered as seconds (e.g. 120) or formats like 1:05 or 1:02:15.</span>
        </div>
        <div class="flex items-center space-x-3">
          <button id="cancel-edit-chapters-btn" class="bg-transparent hover:bg-black-500 text-white px-4 py-2 rounded text-xs transition-colors">
            Cancel
          </button>
          <button id="save-edit-chapters-btn" class="bg-accent text-primary font-bold px-4 py-2 rounded text-xs hover:opacity-90 transition-opacity">
            Save Chapters
          </button>
        </div>
      </div>
    </div>
  `;

  document.body.appendChild(modal);
  renderChaptersList();

  const closeModal = () => modal.remove();
  document.getElementById('close-edit-chapters-modal').onclick = closeModal;
  document.getElementById('cancel-edit-chapters-btn').onclick = closeModal;

  document.getElementById('editor-add-chapter-btn').onclick = () => {
    let nextStart = 0;
    if (currentChapters.length > 0) {
      nextStart = currentChapters[currentChapters.length - 1].end || currentChapters[currentChapters.length - 1].start;
    }
    currentChapters.push({
      title: `New Chapter`,
      start: nextStart,
      end: nextStart + 300
    });
    renderChaptersList();
    
    const listContainer = modal.querySelector('#chapters-editor-list');
    if (listContainer) {
      setTimeout(() => {
        listContainer.scrollTop = listContainer.scrollHeight;
      }, 50);
    }
  };

  document.getElementById('editor-lookup-btn').onclick = async () => {
    const asinVal = item.media?.metadata?.asin;
    if (!asinVal) {
      alert("Book must have an ASIN (under Edit Details) to perform Audnexus chapter lookup.");
      return;
    }

    const btn = document.getElementById('editor-lookup-btn');
    const originalHTML = btn.innerHTML;
    btn.disabled = true;
    btn.innerHTML = `<span class="animate-spin rounded-full h-3.5 w-3.5 border-b-2 border-accent mr-1"></span> Searching...`;

    try {
      const res = await request('POST', `/api/items/${item.id}/chapters/lookup`);
      if (res && Array.isArray(res.chapters) && res.chapters.length > 0) {
        currentChapters = res.chapters;
        renderChaptersList();
      } else {
        alert("Audnexus lookup returned no chapters for this book.");
      }
    } catch (err) {
      console.error("Audnexus lookup failed:", err);
      alert("Audnexus lookup failed: " + (err.message || "Unknown error"));
    } finally {
      btn.disabled = false;
      btn.innerHTML = originalHTML;
    }
  };

  document.getElementById('save-edit-chapters-btn').onclick = async () => {
    for (let i = 0; i < currentChapters.length; i++) {
      const c = currentChapters[i];
      if (!c.title.trim()) {
        alert(`Chapter ${i + 1} title cannot be empty.`);
        return;
      }
      if (c.start < 0 || c.end < 0) {
        alert(`Chapter ${i + 1} times must be non-negative.`);
        return;
      }
      if (c.end <= c.start) {
        alert(`Chapter ${i + 1} end time must be greater than start time.`);
        return;
      }
    }

    currentChapters.sort((a, b) => a.start - b.start);
    currentChapters.forEach((c, idx) => {
      c.id = idx + 1;
    });

    const saveBtn = document.getElementById('save-edit-chapters-btn');
    saveBtn.disabled = true;
    saveBtn.textContent = "Saving...";

    try {
      await request('POST', `/api/items/${item.id}/chapters`, {
        chapters: currentChapters
      });
      closeModal();
      if (onSaveSuccess) onSaveSuccess();
    } catch (err) {
      console.error("Failed to save chapters:", err);
      alert("Failed to save chapters: " + (err.message || "Unknown error"));
      saveBtn.disabled = false;
      saveBtn.textContent = "Save Chapters";
    }
  };
}
