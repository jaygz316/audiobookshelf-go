import { request, resolvePath } from '../api.js';

function escapeHtml(str) {
  if (!str) return '';
  return str.toString()
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&#039;");
}

export async function refreshPdfBookmarksList(itemId, onBookmarkClick) {
  try {
    window.currentUser = await request('GET', '/api/me');
  } catch (e) {
    console.warn("Failed to sync bookmarks:", e);
  }
  
  const curUser = window.currentUser || {};
  const bms = (curUser.bookmarks || []).filter(b => b.libraryItemId === itemId);
  
  bms.sort((a, b) => b.createdAt - a.createdAt);
  
  const container = document.getElementById('pdf-bookmarks-list');
  if (!container) return;
  
  container.innerHTML = `
    ${bms.length === 0 ? `
      <p class="text-[10px] text-black-100 italic text-center py-4">No notes or bookmarks.</p>
    ` : bms.map((b, idx) => {
      const hlColor = b.color || '#ffeb3b';
      const borderStyle = `border-left: 3px solid ${hlColor}; padding-left: 6px;`;
      
      return `
        <div class="bg-black-600/30 hover:bg-black-500/30 border border-black-400/20 rounded p-2 transition-colors relative group cursor-pointer" data-pdf-bm-idx="${idx}" style="${borderStyle}">
          <div class="flex justify-between items-start space-x-1.5">
            <div class="flex-grow min-w-0 pr-4">
              <span class="text-[9px] bg-accent/10 border border-accent/20 text-accent font-semibold px-1 py-0.5 rounded font-mono">Page ${b.time}</span>
              ${b.title ? `<p class="text-[10px] text-white/90 font-medium mt-1 truncate italic">${escapeHtml(b.title)}</p>` : ''}
              ${b.note ? `<p class="text-[9px] text-black-100 mt-1 whitespace-pre-wrap line-clamp-3">${escapeHtml(b.note)}</p>` : ''}
            </div>
            <button class="delete-pdf-bookmark-btn text-black-100 hover:text-error transition-colors p-0.5 rounded hover:bg-black-400 focus:outline-none" data-time="${b.time}" title="Delete bookmark">
              <span class="material-symbols text-xs">delete</span>
            </button>
          </div>
        </div>
      `;
    }).join('')}
  `;

  // Attach event handlers
  container.querySelectorAll('[data-pdf-bm-idx]').forEach(row => {
    const idx = parseInt(row.getAttribute('data-pdf-bm-idx'));
    const b = bms[idx];
    row.onclick = (e) => {
      if (e.target.closest('.delete-pdf-bookmark-btn')) return;
      if (onBookmarkClick) onBookmarkClick(b);
    };
  });

  container.querySelectorAll('.delete-pdf-bookmark-btn').forEach(btn => {
    btn.onclick = async (e) => {
      e.stopPropagation();
      const timeVal = parseFloat(btn.getAttribute('data-time'));
      if (confirm("Are you sure you want to delete this bookmark?")) {
        try {
          await request('DELETE', `/api/me/item/${itemId}/bookmark/${timeVal}`);
          await refreshPdfBookmarksList(itemId, onBookmarkClick);
        } catch (err) {
          console.error("Failed to delete PDF bookmark:", err);
          showToast("Failed to delete bookmark", "error");
        }
      }
    };
  });
}

export async function refreshBookmarksTab(itemId, onBookmarkClick) {
  try {
    window.currentUser = await request('GET', '/api/me');
  } catch (e) {
    console.warn("Failed to sync bookmarks:", e);
  }
  
  const curUser = window.currentUser || {};
  const bms = (curUser.bookmarks || []).filter(b => b.libraryItemId === itemId);
  
  bms.sort((a, b) => b.createdAt - a.createdAt);
  
  const container = document.getElementById('reader-bookmarks-list');
  if (!container) return;
  
  const searchVal = document.getElementById('bookmarks-search-input')?.value || "";
  
  const filteredBms = bms.filter(b => {
    if (!searchVal) return true;
    const q = searchVal.toLowerCase();
    return (b.title || "").toLowerCase().includes(q) || (b.note || "").toLowerCase().includes(q);
  });

  container.innerHTML = `
    <div class="px-2 pb-2">
      <input id="bookmarks-search-input" type="text" placeholder="Search highlights & notes..." class="w-full bg-black-600 border border-black-400 text-white rounded text-xs px-2 py-1.5 focus:outline-none focus:border-accent" value="${escapeHtml(searchVal)}" />
    </div>
    <div class="space-y-2 p-1 max-h-[calc(100vh-12rem)] overflow-y-auto" id="bookmarks-items-container">
      ${filteredBms.length === 0 ? `
        <p class="text-xs text-black-100 italic text-center py-4">No highlights or notes found.</p>
      ` : filteredBms.map((b, idx) => {
        const isHighlight = !!b.cfi;
        const hlColor = b.color || '#ffeb3b';
        const borderStyle = isHighlight ? `border-left: 4px solid ${hlColor}; padding-left: 8px;` : '';
        
        return `
          <div class="bg-black-600/50 hover:bg-black-500/50 border border-black-400/30 rounded p-2.5 transition-colors relative group cursor-pointer" data-idx="${idx}" style="${borderStyle}">
            <div class="flex justify-between items-start space-x-2">
              <div class="flex-grow min-w-0 pr-4">
                ${b.title ? `<p class="text-xs font-semibold text-white/90 line-clamp-3 italic">${escapeHtml(b.title)}</p>` : ''}
                ${b.note ? `<p class="text-[10px] text-black-100 mt-1.5 whitespace-pre-wrap line-clamp-3">${escapeHtml(b.note)}</p>` : ''}
                <div class="flex items-center space-x-2 mt-2">
                  <span class="text-[9px] bg-accent/15 text-accent font-semibold px-1.5 py-0.5 rounded uppercase tracking-wider">${isHighlight ? 'Highlight' : 'Note'}</span>
                  ${b.chapterTitle ? `<span class="text-[9px] text-black-100 truncate max-w-[150px]" title="${escapeHtml(b.chapterTitle)}">${escapeHtml(b.chapterTitle)}</span>` : ''}
                </div>
              </div>
              <button class="delete-bookmark-btn text-black-100 hover:text-error transition-colors p-1 rounded hover:bg-black-400 focus:outline-none" data-time="${b.time}" title="Delete highlight">
                <span class="material-symbols text-sm">delete</span>
              </button>
            </div>
          </div>
        `;
      }).join('')}
    </div>
  `;

  // Attach search listener
  const searchInput = document.getElementById('bookmarks-search-input');
  if (searchInput) {
    let searchTimeout = null;
    searchInput.oninput = () => {
      clearTimeout(searchTimeout);
      searchTimeout = setTimeout(() => {
        refreshBookmarksTab(itemId, onBookmarkClick);
      }, 300);
    };
  }

  // Attach click listeners to rows
  container.querySelectorAll('[data-idx]').forEach(row => {
    const idx = parseInt(row.getAttribute('data-idx'));
    const b = filteredBms[idx];
    row.onclick = (e) => {
      if (e.target.closest('.delete-bookmark-btn')) return;
      if (onBookmarkClick) onBookmarkClick(b);
    };
  });

  // Attach delete buttons listeners
  container.querySelectorAll('.delete-bookmark-btn').forEach(btn => {
    btn.onclick = async (e) => {
      e.stopPropagation();
      const timeVal = parseFloat(btn.getAttribute('data-time'));
      if (confirm("Are you sure you want to delete this highlight?")) {
        try {
          await request('DELETE', `/api/me/item/${itemId}/bookmark/${timeVal}`);
          await refreshBookmarksTab(itemId, onBookmarkClick);
        } catch (err) {
          console.error("Failed to delete bookmark:", err);
          showToast("Failed to delete bookmark", "error");
        }
      }
    };
  });
}
