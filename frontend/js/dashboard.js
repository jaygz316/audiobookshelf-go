// js/dashboard.js

import { request, resolvePath } from './api.js';
import { playItem } from './player.js';

export async function loadDashboard(libraryId) {
  const bookshelfContainer = document.getElementById('bookshelf');
  if (!bookshelfContainer) return;

  // Clear container first
  bookshelfContainer.innerHTML = '';

  // Show loading indicator
  bookshelfContainer.innerHTML = `
    <div class="flex items-center justify-center h-full">
      <div class="animate-spin rounded-full h-8 w-8 border-b-2 border-accent"></div>
    </div>
  `;

  try {
    // 1. Fetch library details to know the mediaType
    const lib = await request('GET', `/api/libraries/${libraryId}`);
    
    // 2. Fetch personalized shelves
    const shelves = await request('GET', `/api/libraries/${libraryId}/personalized`);
    
    // 3. Fetch all items (up to 40)
    const allItemsPayload = await request('GET', `/api/libraries/${libraryId}/items?limit=40&minified=1`);
    
    bookshelfContainer.innerHTML = '';

    const totalItems = allItemsPayload.total || 0;
    
    // Update count in toolbar
    const bookCountEl = document.getElementById('book-count');
    if (bookCountEl) {
      const unit = lib.mediaType === 'podcast' ? 'Podcasts' : 'Books';
      bookCountEl.textContent = `${totalItems} ${unit}`;
    }

    if (shelves.length === 0 && (!allItemsPayload.results || allItemsPayload.results.length === 0)) {
      bookshelfContainer.innerHTML = `
        <div class="flex flex-col items-center justify-center h-48 text-black-100">
          <span class="material-symbols text-4xl mb-2">library_books</span>
          <p class="text-sm font-medium">No items found in this library</p>
        </div>
      `;
      return;
    }

    // Render personalized shelves
    shelves.forEach(shelf => {
      if (shelf.entities && shelf.entities.length > 0) {
        const section = createShelfSection(shelf.id, shelf.label, shelf.entities);
        bookshelfContainer.appendChild(section);
      }
    });

    // Render "All Books" / "All Podcasts" shelf
    if (allItemsPayload.results && allItemsPayload.results.length > 0) {
      const allLabel = lib.mediaType === 'podcast' ? 'All Podcasts' : 'All Books';
      const section = createShelfSection('all-books', allLabel, allItemsPayload.results);
      bookshelfContainer.appendChild(section);
    }

  } catch (err) {
    console.error('Failed to load dashboard:', err);
    bookshelfContainer.innerHTML = `
      <div class="flex flex-col items-center justify-center h-48 text-red-400">
        <span class="material-symbols text-4xl mb-2">error</span>
        <p class="text-sm font-medium">Failed to load library items</p>
      </div>
    `;
  }
}

function createShelfSection(shelfId, label, entities) {
  const shelfWrapper = document.createElement('div');
  shelfWrapper.className = 'relative w-full';
  
  const rowDiv = document.createElement('div');
  rowDiv.className = 'w-full h-56 relative overflow-x-auto no-scroll overflow-y-hidden z-10 bg-repeat-x bookshelfRow';
  
  const itemsContainer = document.createElement('div');
  itemsContainer.id = `${shelfId}-shelf`;
  itemsContainer.className = 'w-max h-full pt-4e flex items-center pl-8e pr-8e';
  
  entities.forEach(item => {
    const card = createCard(item, shelfId.startsWith('continue'));
    itemsContainer.appendChild(card);
  });
  
  rowDiv.appendChild(itemsContainer);
  shelfWrapper.appendChild(rowDiv);
  
  const plaqueDiv = document.createElement('div');
  plaqueDiv.className = 'relative h-12';
  plaqueDiv.innerHTML = `
    <div class="relative text-center categoryPlacard z-30 top-0 w-44e rounded-md mx-auto">
      <div class="shinyBlack flex items-center justify-center border rounded px-2 py-1">
        <h3 class="text-[0.85em] font-semibold tracking-wider font-mono">${label.toUpperCase()}</h3>
      </div>
    </div>
    <div class="bookshelfDividerCategorized h-6e w-full absolute top-0 left-0 right-0 z-20"></div>
  `;
  shelfWrapper.appendChild(plaqueDiv);
  
  return shelfWrapper;
}

function createCard(item, isContinue) {
  const card = document.createElement('div');
  card.className = 'w-28e h-40e mr-8e relative cursor-pointer select-none box-shadow-book rounded-sm overflow-hidden flex-shrink-0 transition-transform hover:scale-105 group';
  
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

  card.innerHTML = `
    <img class="w-full h-full object-cover" src="${coverUrl}" alt="${escapeHtml(title)}" onerror="this.onerror=null; this.src='assets/images/logo.png'">
    
    <!-- Hover overlay details -->
    <div class="absolute inset-0 bg-black/85 opacity-0 group-hover:opacity-100 transition-opacity duration-200 flex flex-col justify-between p-3 select-none text-left z-30">
      <div class="overflow-y-auto no-scroll">
        <h4 class="font-semibold text-sm text-white leading-tight mb-1">${escapeHtml(title)}</h4>
        <p class="text-xs text-black-100">${escapeHtml(author)}</p>
      </div>
    </div>
  `;

  if (isContinue) {
    const progBarContainer = document.createElement('div');
    progBarContainer.className = 'absolute bottom-0 left-0 right-0 h-1.5 bg-black/40 box-shadow-progressbar rounded-b-sm overflow-hidden z-20 hidden';
    const progBarFill = document.createElement('div');
    progBarFill.className = 'h-full bg-accent';
    progBarFill.style.width = '0%';
    progBarContainer.appendChild(progBarFill);
    card.appendChild(progBarContainer);

    // Fetch progress asynchronously
    request('GET', `/api/me/progress/${item.id}`)
      .then(progressObj => {
        if (progressObj && progressObj.progress !== undefined) {
          const percent = Math.min(Math.max(progressObj.progress * 100, 0), 100);
          progBarFill.style.width = `${percent}%`;
          progBarContainer.classList.remove('hidden');
          
          const overlayDiv = card.querySelector('.group-hover\\:opacity-100');
          if (overlayDiv) {
            const container = overlayDiv.querySelector('div');
            if (container) {
              const progressText = document.createElement('p');
              progressText.className = 'text-xs text-accent mt-2';
              progressText.textContent = `${Math.round(percent)}% completed`;
              container.appendChild(progressText);
            }
          }
        }
      })
      .catch(err => {
        console.warn(`Failed to fetch progress for item ${item.id}:`, err);
      });
  }

  // Click handler to trigger playback
  card.addEventListener('click', async () => {
    try {
      let startTime = 0;
      try {
        const progressObj = await request('GET', `/api/me/progress/${item.id}`);
        if (progressObj && progressObj.currentTime !== undefined) {
          startTime = progressObj.currentTime;
        }
      } catch (err) {
        // Safe to ignore, starts at 0
      }
      playItem(item, startTime);
    } catch (err) {
      console.error('Failed to start playback:', err);
    }
  });

  return card;
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
