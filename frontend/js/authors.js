// frontend/js/authors.js
// Provides Authors and Series listing views for the sidebar navigation.

import { request, resolvePath } from './api.js';
import { getActiveLibraryId } from './library.js';
import { showToast } from './app.js';
import { createCard } from './dashboard.js';

/**
 * Load and render the Authors listing view for the given library.
 * Fetches GET /api/libraries/{libraryId}/authors and renders a grid.
 */
export async function loadAuthors(libraryId) {
  const opmlBtn = document.getElementById('opml-btn');
  if (opmlBtn) opmlBtn.classList.add('hidden');

  const container = document.getElementById('bookshelf');
  if (!container) return;

  const viewTitle = document.getElementById('view-title');
  if (viewTitle) viewTitle.textContent = 'Authors';

  const bookCount = document.getElementById('book-count');
  if (bookCount) bookCount.textContent = '';

  container.innerHTML = `
    <div class="flex items-center justify-center h-32">
      <div class="animate-spin rounded-full h-8 w-8 border-b-2 border-accent"></div>
    </div>
  `;

  try {
    const payload = await request('GET', `/api/libraries/${libraryId}/authors`);
    const authors = payload.authors || [];

    if (bookCount) bookCount.textContent = `${authors.length} Authors`;

    if (authors.length === 0) {
      container.innerHTML = `
        <div class="flex flex-col items-center justify-center h-48 text-black-100">
          <span class="material-symbols text-4xl mb-2">person</span>
          <p class="text-sm font-medium">No authors found in this library</p>
        </div>
      `;
      return;
    }

    container.innerHTML = '';

    const grid = document.createElement('div');
    grid.className = 'grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 gap-4 p-6';

    authors.forEach(author => {
      const card = createAuthorCard(author);
      grid.appendChild(card);
    });

    container.appendChild(grid);
  } catch (err) {
    console.error('Failed to load authors:', err);
    container.innerHTML = `
      <div class="flex flex-col items-center justify-center h-48 text-red-400">
        <span class="material-symbols text-4xl mb-2">error</span>
        <p class="text-sm font-medium">Failed to load authors: ${escapeHtml(err.message)}</p>
      </div>
    `;
  }
}

/**
 * Load and render the Series listing view for the given library.
 * Fetches GET /api/libraries/{libraryId}/series and renders a grid.
 */
export async function loadSeries(libraryId) {
  const opmlBtn = document.getElementById('opml-btn');
  if (opmlBtn) opmlBtn.classList.add('hidden');

  const container = document.getElementById('bookshelf');
  if (!container) return;

  const viewTitle = document.getElementById('view-title');
  if (viewTitle) viewTitle.textContent = 'Series';

  const bookCount = document.getElementById('book-count');
  if (bookCount) bookCount.textContent = '';

  container.innerHTML = `
    <div class="flex items-center justify-center h-32">
      <div class="animate-spin rounded-full h-8 w-8 border-b-2 border-accent"></div>
    </div>
  `;

  try {
    const payload = await request('GET', `/api/libraries/${libraryId}/series`);
    const seriesList = payload.results || payload.series || [];

    if (bookCount) bookCount.textContent = `${seriesList.length} Series`;

    if (seriesList.length === 0) {
      container.innerHTML = `
        <div class="flex flex-col items-center justify-center h-48 text-black-100">
          <span class="material-symbols text-4xl mb-2">layers</span>
          <p class="text-sm font-medium">No series found in this library</p>
        </div>
      `;
      return;
    }

    container.innerHTML = '';

    const grid = document.createElement('div');
    grid.className = 'grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 gap-4 p-6';

    seriesList.forEach(series => {
      const card = createSeriesCard(series);
      grid.appendChild(card);
    });

    container.appendChild(grid);
  } catch (err) {
    console.error('Failed to load series:', err);
    container.innerHTML = `
      <div class="flex flex-col items-center justify-center h-48 text-red-400">
        <span class="material-symbols text-4xl mb-2">error</span>
        <p class="text-sm font-medium">Failed to load series: ${escapeHtml(err.message)}</p>
      </div>
    `;
  }
}

function createAuthorCard(author) {
  const card = document.createElement('div');
  card.className = 'flex flex-col items-center p-3 bg-primary border border-black-400 rounded-md hover:bg-black-500 cursor-pointer transition-colors group';

  const token = localStorage.getItem('token');
  const imageUrl = resolvePath(`/api/authors/${author.id}/image?token=${token}`);

  const numBooks = author.numBooks !== undefined ? author.numBooks : (author.bookCount || 0);

  card.innerHTML = `
    <div class="w-20 h-20 rounded-full overflow-hidden bg-black-400 mb-2 flex items-center justify-center flex-shrink-0">
      <img src="${imageUrl}" alt="${escapeHtml(author.name)}" class="w-full h-full object-cover">
    </div>
    <p class="text-sm font-semibold text-white text-center leading-tight truncate w-full text-center">${escapeHtml(author.name)}</p>
    <p class="text-xs text-black-100 mt-0.5">${numBooks} book${numBooks !== 1 ? 's' : ''}</p>
  `;

  const img = card.querySelector('img');
  if (img) {
    img.addEventListener('error', function() {
      const parent = this.parentElement;
      if (parent) {
        parent.innerHTML = '<span class="material-symbols text-4xl text-black-100">person</span>';
      }
    });
  }

  card.onclick = () => {
    window.dispatchEvent(new CustomEvent('navigate-to-author', {
      detail: {
        authorId: author.id,
        authorName: author.name
      }
    }));
  };

  return card;
}

function createSeriesCard(series) {
  const card = document.createElement('div');
  card.className = 'flex flex-col items-center p-4 bg-primary/45 border border-black-400/40 rounded-lg hover:bg-black-500/50 cursor-pointer transition-all duration-300 relative select-none hover:-translate-y-1 shadow-lg group';

  const numBooks = series.numBooks !== undefined ? series.numBooks : (series.books?.length || 0);

  let coversHtml = '';
  const token = localStorage.getItem('token');

  if (series.books && series.books.length > 0) {
    const books = series.books;
    if (books.length === 1) {
      const ts = books[0].updatedAt || books[0].addedAt || Date.now();
      const coverUrl = resolvePath(`/api/items/${books[0].id}/cover?token=${token}&ts=${ts}`);
      coversHtml = `
        <div class="relative w-24 h-24 mb-3 flex items-center justify-center flex-shrink-0">
          <img src="${coverUrl}" class="w-16 h-22 object-cover rounded shadow-md transition-transform duration-300 group-hover:scale-105" onerror="this.onerror=null; this.src='assets/images/logo.png'">
        </div>
      `;
    } else if (books.length === 2) {
      const ts0 = books[0].updatedAt || books[0].addedAt || Date.now();
      const ts1 = books[1].updatedAt || books[1].addedAt || Date.now();
      const cover0 = resolvePath(`/api/items/${books[0].id}/cover?token=${token}&ts=${ts0}`);
      const cover1 = resolvePath(`/api/items/${books[1].id}/cover?token=${token}&ts=${ts1}`);
      coversHtml = `
        <div class="relative w-24 h-24 mb-3 flex items-center justify-center flex-shrink-0">
          <img src="${cover1}" class="absolute w-14 h-20 object-cover rounded shadow-md transform -translate-x-2.5 rotate-[-8deg] z-10 opacity-80" onerror="this.onerror=null; this.src='assets/images/logo.png'">
          <img src="${cover0}" class="absolute w-15 h-21 object-cover rounded shadow-lg z-20 transition-transform duration-300 group-hover:scale-105" onerror="this.onerror=null; this.src='assets/images/logo.png'">
        </div>
      `;
    } else {
      const ts0 = books[0].updatedAt || books[0].addedAt || Date.now();
      const ts1 = books[1].updatedAt || books[1].addedAt || Date.now();
      const ts2 = books[2].updatedAt || books[2].addedAt || Date.now();
      const cover0 = resolvePath(`/api/items/${books[0].id}/cover?token=${token}&ts=${ts0}`);
      const cover1 = resolvePath(`/api/items/${books[1].id}/cover?token=${token}&ts=${ts1}`);
      const cover2 = resolvePath(`/api/items/${books[2].id}/cover?token=${token}&ts=${ts2}`);
      coversHtml = `
        <div class="relative w-24 h-24 mb-3 flex items-center justify-center flex-shrink-0">
          <img src="${cover2}" class="absolute w-13 h-19 object-cover rounded shadow-md transform -translate-x-4.5 rotate-[-15deg] z-10 opacity-70" onerror="this.onerror=null; this.src='assets/images/logo.png'">
          <img src="${cover1}" class="absolute w-13 h-19 object-cover rounded shadow-md transform translate-x-4.5 rotate-[15deg] z-10 opacity-70" onerror="this.onerror=null; this.src='assets/images/logo.png'">
          <img src="${cover0}" class="absolute w-15 h-21 object-cover rounded shadow-lg z-20 transition-transform duration-300 group-hover:scale-105" onerror="this.onerror=null; this.src='assets/images/logo.png'">
        </div>
      `;
    }
  } else {
    coversHtml = `
      <div class="w-24 h-24 mb-3 flex items-center justify-center bg-black-400/30 rounded-md flex-shrink-0 text-black-100">
        <span class="material-symbols text-4xl">layers</span>
      </div>
    `;
  }

  card.innerHTML = `
    <div class="absolute top-2 right-2 bg-accent/90 text-primary text-[10px] font-bold px-1.5 py-0.5 rounded-full z-30 shadow-sm border border-accent/20">
      ${numBooks}
    </div>
    ${coversHtml}
    <p class="text-sm font-semibold text-white text-center leading-tight truncate w-full mt-1 group-hover:text-accent transition-colors duration-200">${escapeHtml(series.name)}</p>
    <p class="text-xs text-black-100 mt-1">${numBooks} book${numBooks !== 1 ? 's' : ''}</p>
  `;

  card.onclick = () => {
    window.dispatchEvent(new CustomEvent('navigate-to-series', {
      detail: {
        seriesId: series.id,
        seriesName: series.name
      }
    }));
  };

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

export async function loadAuthorDetails(authorId) {
  const container = document.getElementById('bookshelf');
  if (!container) return;

  const libraryId = getActiveLibraryId();
  const viewTitle = document.getElementById('view-title');
  if (viewTitle) viewTitle.textContent = 'Author Details';

  container.innerHTML = `
    <div class="flex items-center justify-center h-32">
      <div class="animate-spin rounded-full h-8 w-8 border-b-2 border-accent"></div>
    </div>
  `;

  try {
    const author = await request('GET', `/api/authors/${authorId}?include=items,series`);
    
    const name = author.name || 'Unknown Author';
    const lastFirst = author.lastFirst || '';
    const asin = author.asin || '';
    const description = author.description || '';
    const token = localStorage.getItem('token');
    const imageUrl = resolvePath(`/api/authors/${author.id}/image?token=${token}`);

    let html = `
      <div class="p-6 max-w-6xl mx-auto space-y-8 text-left">
        <!-- Author Info Header -->
        <div class="flex flex-col md:flex-row gap-6 bg-black-600 p-6 rounded-lg border border-black-400">
          <div class="w-32 h-32 md:w-40 md:h-40 rounded-full overflow-hidden bg-black-400 flex items-center justify-center flex-shrink-0 mx-auto md:mx-0">
            <img id="author-detail-img" src="${imageUrl}" alt="${escapeHtml(name)}" class="w-full h-full object-cover">
          </div>
          <div class="flex-grow flex flex-col justify-between text-center md:text-left">
            <div>
              <div class="flex flex-col md:flex-row md:items-center gap-3 justify-center md:justify-start">
                <h2 class="text-3xl font-bold text-white">${escapeHtml(name)}</h2>
                ${window.currentUser?.type === 'root' || window.currentUser?.type === 'admin' ? `
                  <button id="edit-author-btn" class="px-3 py-1 bg-accent/20 text-accent hover:bg-accent/30 border border-accent/40 rounded text-xs font-semibold self-center transition-colors">
                    Edit
                  </button>
                  <button id="match-author-direct-btn" class="px-3 py-1 bg-accent/20 text-accent hover:bg-accent/30 border border-accent/40 rounded text-xs font-semibold self-center transition-colors ml-2">
                    Match
                  </button>
                ` : ''}
              </div>
              ${lastFirst ? `<p class="text-sm text-black-100 mt-1">Sorting Name: ${escapeHtml(lastFirst)}</p>` : ''}
              ${asin ? `<p class="text-sm text-black-100 mt-1">ASIN: ${escapeHtml(asin)}</p>` : ''}
            </div>
            ${description ? `
              <div class="mt-4 text-sm text-black-50 leading-relaxed max-w-3xl">
                ${escapeHtml(description)}
              </div>
            ` : `<p class="text-sm text-black-200 italic mt-4">No biography available.</p>`}
          </div>
        </div>

        <!-- Series / Books sections -->
        <div class="space-y-8" id="author-books-container">
        </div>
      </div>
    `;

    container.innerHTML = html;

    const detailImg = document.getElementById('author-detail-img');
    if (detailImg) {
      detailImg.addEventListener('error', function() {
        const parent = this.parentElement;
        if (parent) {
          parent.innerHTML = '<span class="material-symbols text-6xl text-black-100">person</span>';
        }
      });
    }

    const booksContainer = document.getElementById('author-books-container');
    const allItems = author.libraryItems || [];
    const seriesList = author.series || [];

    const itemIdsInSeries = new Set();
    seriesList.forEach(s => {
      (s.items || []).forEach(item => {
        itemIdsInSeries.add(item.id);
      });
    });

    seriesList.forEach(s => {
      const sDiv = document.createElement('div');
      sDiv.className = 'space-y-3';
      
      const header = document.createElement('div');
      header.className = 'flex items-center space-x-2 border-b border-black-400 pb-2 cursor-pointer hover:text-accent group';
      header.innerHTML = `
        <span class="material-symbols text-xl text-accent">layers</span>
        <h3 class="text-xl font-bold text-white group-hover:text-accent">${escapeHtml(s.name)}</h3>
        <span class="text-xs text-black-100">(${s.items?.length || 0} book${s.items?.length !== 1 ? 's' : ''})</span>
      `;
      header.onclick = () => {
        window.dispatchEvent(new CustomEvent('navigate-to-series', {
          detail: {
            seriesId: s.id,
            seriesName: s.name
          }
        }));
      };
      sDiv.appendChild(header);

      const itemsGrid = document.createElement('div');
      itemsGrid.className = 'flex flex-wrap gap-4 pt-2';

      const sortedItems = [...(s.items || [])].sort((a, b) => {
        const seqA = parseFloat(a.sequence) || 0;
        const seqB = parseFloat(b.sequence) || 0;
        return seqA - seqB;
      });

      sortedItems.forEach(item => {
        const card = createCard(item, false, libraryId);
        if (item.sequence) {
          const badge = document.createElement('div');
          badge.className = 'absolute top-1 left-1 bg-black/80 text-accent text-[10px] px-1.5 py-0.5 rounded font-semibold z-40';
          badge.textContent = `Book ${item.sequence}`;
          card.appendChild(badge);
        }
        itemsGrid.appendChild(card);
      });

      sDiv.appendChild(itemsGrid);
      booksContainer.appendChild(sDiv);
    });

    const standaloneBooks = allItems.filter(item => !itemIdsInSeries.has(item.id));
    if (standaloneBooks.length > 0) {
      const sDiv = document.createElement('div');
      sDiv.className = 'space-y-3';
      sDiv.innerHTML = `
        <div class="flex items-center space-x-2 border-b border-black-400 pb-2">
          <span class="material-symbols text-xl text-accent">book</span>
          <h3 class="text-xl font-bold text-white">Other Books</h3>
          <span class="text-xs text-black-100">(${standaloneBooks.length} book${standaloneBooks.length !== 1 ? 's' : ''})</span>
        </div>
      `;
      const itemsGrid = document.createElement('div');
      itemsGrid.className = 'flex flex-wrap gap-4 pt-2';
      standaloneBooks.forEach(item => {
        const card = createCard(item, false, libraryId);
        itemsGrid.appendChild(card);
      });
      sDiv.appendChild(itemsGrid);
      booksContainer.appendChild(sDiv);
    }

    const editBtn = document.getElementById('edit-author-btn');
    if (editBtn) {
      editBtn.onclick = () => openEditAuthorModal(author);
    }

    const matchBtn = document.getElementById('match-author-direct-btn');
    if (matchBtn) {
      matchBtn.onclick = () => openMatchAuthorModal(author, null);
    }


  } catch (err) {
    console.error('Failed to load author details:', err);
    container.innerHTML = `
      <div class="flex flex-col items-center justify-center h-48 text-red-400">
        <span class="material-symbols text-4xl mb-2">error</span>
        <p class="text-sm font-medium">Failed to load author: ${escapeHtml(err.message)}</p>
      </div>
    `;
  }
}

export async function loadSeriesDetails(seriesId) {
  const container = document.getElementById('bookshelf');
  if (!container) return;

  const libraryId = getActiveLibraryId();
  const viewTitle = document.getElementById('view-title');
  if (viewTitle) viewTitle.textContent = 'Series Details';

  container.innerHTML = `
    <div class="flex items-center justify-center h-32">
      <div class="animate-spin rounded-full h-8 w-8 border-b-2 border-accent"></div>
    </div>
  `;

  try {
    const series = await request('GET', `/api/libraries/${libraryId}/series/${seriesId}`);
    const payload = await request('GET', `/api/libraries/${libraryId}/items?filter=series.${seriesId}&sort=sequence`);
    const items = payload.results || [];

    const name = series.name || 'Unknown Series';
    const description = series.description || '';
    const progress = series.progress || {};
    const totalBooks = items.length;
    const finishedCount = progress.libraryItemIdsFinished?.length || 0;
    
    let html = `
      <div class="p-6 max-w-6xl mx-auto space-y-8 text-left">
        <!-- Series Info Header -->
        <div class="bg-black-600 p-6 rounded-lg border border-black-400 space-y-4">
          <div class="flex flex-col md:flex-row md:items-center gap-3 justify-between">
            <div class="flex items-center space-x-3">
              <span class="material-symbols text-3xl text-accent">layers</span>
              <h2 class="text-3xl font-bold text-white">${escapeHtml(name)}</h2>
              ${window.currentUser?.type === 'root' || window.currentUser?.type === 'admin' ? `
                <div class="flex items-center space-x-2">
                  <button id="edit-series-btn" class="px-3 py-1 bg-accent/20 text-accent hover:bg-accent/30 border border-accent/40 rounded text-xs font-semibold self-center transition-colors">
                    Edit
                  </button>
                  <button id="auto-number-series-btn" class="px-3 py-1 bg-accent/20 text-accent hover:bg-accent/30 border border-accent/40 rounded text-xs font-semibold self-center transition-colors flex items-center space-x-1">
                    <span class="material-symbols text-[13px]">format_list_numbered</span>
                    <span>Auto-Number</span>
                  </button>
                </div>
              ` : ''}
            </div>
            
            <!-- Progress stats -->
            <div class="text-sm bg-primary/60 px-4 py-2 rounded-md border border-black-400 flex items-center space-x-3">
              <span class="font-medium text-black-50">${totalBooks} book${totalBooks !== 1 ? 's' : ''}</span>
              <span class="text-black-300">|</span>
              <span class="font-medium text-accent">${finishedCount} completed</span>
            </div>
          </div>

          ${description ? `
            <div class="text-sm text-black-50 leading-relaxed max-w-3xl border-t border-black-400/40 pt-3">
              ${escapeHtml(description)}
            </div>
          ` : `<p class="text-sm text-black-200 italic border-t border-black-400/40 pt-3">No description available.</p>`}
        </div>

        <!-- Books List -->
        <div class="space-y-6">
          <div class="border-b border-black-400 pb-2 flex items-center justify-between">
            <h3 class="text-xl font-bold text-white">Series Matrix</h3>
            <span class="text-xs text-black-300">Chronological list of books and editions</span>
          </div>
          <div id="series-matrix-container" class="space-y-4 pt-2">
            ${items.length === 0 ? `
              <p class="text-sm text-black-200 italic">No books in this series yet.</p>
            ` : ''}
          </div>
        </div>
      </div>
    `;

    container.innerHTML = html;

    const matrixContainer = container.querySelector('#series-matrix-container');
    if (matrixContainer && items.length > 0) {
      // Group items by sequence
      const groups = {};
      items.forEach(item => {
        let seq = '';
        if (item.media?.metadata?.series?.sequence) {
          seq = item.media.metadata.series.sequence;
        } else if (item.media?.metadata?.seriesName === name && item.media?.metadata?.seriesSequence) {
          seq = item.media.metadata.seriesSequence;
        } else if (item.media?.metadata?.series) {
          const matchingSeries = (item.media.metadata.series || []).find(s => s.name === name || s.id === seriesId);
          if (matchingSeries && matchingSeries.sequence) {
            seq = matchingSeries.sequence;
          }
        }
        if (!seq) {
          seq = 'Unsequenced';
        }
        if (!groups[seq]) {
          groups[seq] = [];
        }
        groups[seq].push(item);
      });

      // Sort sequences: numbers first, then 'Unsequenced'
      const sortedSeqs = Object.keys(groups).sort((a, b) => {
        if (a === 'Unsequenced') return 1;
        if (b === 'Unsequenced') return -1;
        const valA = parseFloat(a) || 0;
        const valB = parseFloat(b) || 0;
        return valA - valB;
      });

      matrixContainer.innerHTML = ''; // clear initial msg

      sortedSeqs.forEach(seq => {
        const groupItems = groups[seq];
        const row = document.createElement('div');
        row.className = 'flex flex-col md:flex-row md:items-stretch bg-black-600/30 border border-black-400/40 rounded-lg overflow-hidden transition-all hover:border-accent/30 mb-4';
        
        // Sequence side banner
        const sideBanner = document.createElement('div');
        sideBanner.className = 'w-full md:w-32 bg-black-600/50 p-4 border-b md:border-b-0 md:border-r border-black-400/40 flex flex-col justify-center items-center text-center';
        
        let seqLabel = seq;
        if (seq !== 'Unsequenced') {
          seqLabel = `Book ${seq}`;
        }
        
        sideBanner.innerHTML = `
          <span class="text-accent text-sm font-semibold tracking-wider uppercase">${seqLabel}</span>
          <span class="text-xs text-black-300 mt-1">${groupItems.length} version${groupItems.length !== 1 ? 's' : ''}</span>
        `;
        row.appendChild(sideBanner);

        // Versions container
        const versionsContainer = document.createElement('div');
        versionsContainer.className = 'flex-1 p-4 flex flex-wrap gap-6 items-center';
        
        groupItems.forEach(item => {
          const cardContainer = document.createElement('div');
          cardContainer.className = 'relative flex items-center space-x-4 bg-black-500/20 p-3 rounded-lg border border-black-400/20 max-w-sm w-full hover:border-black-400/60 transition-colors';
          
          const card = createCard(item, false, libraryId);
          card.classList.add('flex-shrink-0');
          
          cardContainer.appendChild(card);

          // Build metadata block for version comparison
          const infoBlock = document.createElement('div');
          infoBlock.className = 'flex-1 min-w-0 text-left';
          
          const metadata = item.media?.metadata || {};
          const title = metadata.title || item.title || 'Untitled';
          const narrator = metadata.narratorName || '';
          const publisher = metadata.publisher || '';
          const duration = item.media?.duration || 0;
          
          let durationStr = 'Unknown duration';
          if (duration > 0) {
            const hrs = Math.floor(duration / 3600);
            const mins = Math.floor((duration % 3600) / 60);
            if (hrs > 0) {
              durationStr = `${hrs}h ${mins}m`;
            } else {
              durationStr = `${mins}m`;
            }
          }

          infoBlock.innerHTML = `
            <h4 class="font-bold text-sm text-white truncate" title="${escapeHtml(title)}">${escapeHtml(title)}</h4>
            ${narrator ? `<p class="text-xs text-accent mt-1 truncate" title="${escapeHtml(narrator)}"><span class="text-black-300 font-medium">Narrator:</span> ${escapeHtml(narrator)}</p>` : '<p class="text-xs text-black-300 mt-1 italic">No narrator info</p>'}
            ${publisher ? `<p class="text-[11px] text-black-200 truncate" title="${escapeHtml(publisher)}"><span class="text-black-400">Publisher:</span> ${escapeHtml(publisher)}</p>` : ''}
            <p class="text-[11px] text-black-300 mt-1 flex items-center space-x-1.5">
              <span class="material-symbols text-[13px] text-black-400">schedule</span>
              <span>${durationStr}</span>
            </p>
          `;
          cardContainer.appendChild(infoBlock);
          versionsContainer.appendChild(cardContainer);
        });

        row.appendChild(versionsContainer);
        matrixContainer.appendChild(row);
      });
    }

    const editBtn = document.getElementById('edit-series-btn');
    if (editBtn) {
      editBtn.onclick = () => openEditSeriesModal(series);
    }

    const autoNumberBtn = document.getElementById('auto-number-series-btn');
    if (autoNumberBtn) {
      autoNumberBtn.onclick = async () => {
        if (!confirm('Are you sure you want to automatically number all books in this series chronologically? This will overwrite existing sequences.')) {
          return;
        }
        try {
          autoNumberBtn.disabled = true;
          autoNumberBtn.innerHTML = `
            <div class="animate-spin rounded-full h-3 w-3 border border-t-transparent border-accent mr-1"></div>
            <span>Auto-Numbering...</span>
          `;
          await request('POST', `/api/series/${series.id}/auto-number`);
          showToast('Series auto-numbered successfully', 'success');
          loadSeriesDetails(series.id);
        } catch (err) {
          showToast('Failed to auto-number: ' + err.message, 'error');
          autoNumberBtn.disabled = false;
          autoNumberBtn.innerHTML = `
            <span class="material-symbols text-[13px]">format_list_numbered</span>
            <span>Auto-Number</span>
          `;
        }
      };
    }

  } catch (err) {
    console.error('Failed to load series details:', err);
    container.innerHTML = `
      <div class="flex flex-col items-center justify-center h-48 text-red-400">
        <span class="material-symbols text-4xl mb-2">error</span>
        <p class="text-sm font-medium">Failed to load series: ${escapeHtml(err.message)}</p>
      </div>
    `;
  }
}

function openEditAuthorModal(author) {
  const modal = document.createElement('div');
  modal.className = 'fixed inset-0 bg-black-900/80 z-50 flex items-center justify-center p-4';
  
  modal.innerHTML = `
    <div class="bg-black-600 border border-black-400 rounded-lg max-w-lg w-full flex flex-col overflow-hidden max-h-[90vh] text-left">
      <div class="px-6 py-4 border-b border-black-400 flex items-center justify-between">
        <h3 class="text-lg font-bold text-white">Edit Author</h3>
        <button id="close-author-modal" class="text-black-100 hover:text-white font-bold text-xl">&times;</button>
      </div>
      <div class="p-6 overflow-y-auto space-y-4">
        <div>
          <label class="block text-xs uppercase font-semibold text-black-100 mb-1.5">Name</label>
          <input type="text" id="edit-author-name" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent text-xs" value="${escapeHtml(author.name)}">
        </div>
        <div>
          <label class="block text-xs uppercase font-semibold text-black-100 mb-1.5">Sorting Name (Last First)</label>
          <input type="text" id="edit-author-lastfirst" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent text-xs" value="${escapeHtml(author.lastFirst)}">
        </div>
        <div>
          <label class="block text-xs uppercase font-semibold text-black-100 mb-1.5">ASIN</label>
          <div class="flex space-x-2">
            <input type="text" id="edit-author-asin" class="flex-grow bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent text-xs" value="${escapeHtml(author.asin || '')}">
            <button id="match-author-btn" class="bg-accent hover:bg-accent-hover text-black px-4 py-2 rounded text-xs font-semibold whitespace-nowrap">Match</button>
          </div>
        </div>
        <div>
          <label class="block text-xs uppercase font-semibold text-black-100 mb-1.5">Biography</label>
          <textarea id="edit-author-description" rows="5" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent text-xs">${escapeHtml(author.description || '')}</textarea>
        </div>
        ${author.imagePath ? `
        <div class="flex items-center justify-between bg-black-500/30 p-3 rounded border border-black-300">
          <div class="flex items-center space-x-3">
            <img src="${resolvePath(`/api/authors/${author.id}/image?token=${localStorage.getItem('token')}`)}" class="w-12 h-12 rounded-full object-cover">
            <span class="text-xs text-white font-medium">Author Photo</span>
          </div>
          <button id="remove-author-image-btn" class="bg-red-950/40 hover:bg-red-900 border border-red-500/50 text-red-200 text-xs px-3 py-1.5 rounded transition-colors font-semibold">Remove Image</button>
        </div>
        ` : ''}
      </div>
      <div class="px-6 py-4 border-t border-black-400 flex justify-end space-x-3">
        <button id="cancel-author-edit" class="bg-black-400 hover:bg-black-300 text-white px-4 py-2 rounded text-xs font-semibold">Cancel</button>
        <button id="save-author-edit" class="bg-accent hover:bg-accent-hover text-black px-4 py-2 rounded text-xs font-semibold">Save</button>
      </div>
    </div>
  `;

  document.body.appendChild(modal);

  const closeModal = () => modal.remove();
  document.getElementById('close-author-modal').onclick = closeModal;
  document.getElementById('cancel-author-edit').onclick = closeModal;

  document.getElementById('match-author-btn').onclick = (e) => {
    e.preventDefault();
    openMatchAuthorModal(author, closeModal);
  };

  const removeImgBtn = document.getElementById('remove-author-image-btn');
  if (removeImgBtn) {
    removeImgBtn.onclick = async (e) => {
      e.preventDefault();
      if (!confirm('Are you sure you want to remove the author image?')) return;
      try {
        await request('DELETE', `/api/authors/${author.id}/image`);
        showToast('Author image removed successfully', 'success');
        closeModal();
        loadAuthorDetails(author.id);
      } catch (err) {
        showToast('Failed to remove image: ' + err.message, 'error');
      }
    };
  };

  document.getElementById('save-author-edit').onclick = async () => {
    const name = document.getElementById('edit-author-name').value.trim();
    const lastFirst = document.getElementById('edit-author-lastfirst').value.trim();
    const asin = document.getElementById('edit-author-asin').value.trim();
    const description = document.getElementById('edit-author-description').value.trim();

    if (!name) {
      showToast('Name is required', 'error');
      return;
    }

    try {
      await request('PATCH', `/api/authors/${author.id}`, {
        name,
        lastFirst,
        asin,
        description
      });
      showToast('Author updated successfully', 'success');
      closeModal();
      loadAuthorDetails(author.id);
    } catch (err) {
      showToast('Failed to update author: ' + err.message, 'error');
    }
  };
}

function openMatchAuthorModal(author, parentCloseModal) {
  const modal = document.createElement('div');
  modal.className = 'fixed inset-0 bg-black-900/80 z-[60] flex items-center justify-center p-4';
  
  modal.innerHTML = `
    <div class="bg-black-600 border border-black-400 rounded-lg max-w-2xl w-full flex flex-col overflow-hidden max-h-[90vh] text-left">
      <div class="px-6 py-4 border-b border-black-400 flex items-center justify-between">
        <h3 class="text-lg font-bold text-white">Match Author</h3>
        <button id="close-match-modal" class="text-black-100 hover:text-white font-bold text-xl">&times;</button>
      </div>
      <div class="p-6 overflow-y-auto space-y-4 flex-grow">
        <div class="flex space-x-3 items-end">
          <div class="flex-grow">
            <label class="block text-xs uppercase font-semibold text-black-100 mb-1.5">Search Author Name</label>
            <input type="text" id="match-search-query" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent text-xs" value="${escapeHtml(author.name)}">
          </div>
          <div>
            <label class="block text-xs uppercase font-semibold text-black-100 mb-1.5">Provider</label>
            <select id="match-provider" class="bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent text-xs h-[34px]">
              <option value="audnexus">Audnexus</option>
            </select>
          </div>
          <button id="match-search-btn" class="bg-accent hover:bg-accent-hover text-black px-5 py-2 rounded text-xs font-semibold h-[34px]">Search</button>
        </div>

        <div id="match-results-container" class="space-y-3 pt-4 border-t border-black-400 overflow-y-auto max-h-[50vh]">
          <p class="text-black-100 text-xs italic">Enter search query and click search.</p>
        </div>
      </div>
      <div class="px-6 py-4 border-t border-black-400 flex justify-end">
        <button id="cancel-match" class="bg-black-400 hover:bg-black-300 text-white px-4 py-2 rounded text-xs font-semibold">Close</button>
      </div>
    </div>
  `;

  document.body.appendChild(modal);

  const closeMatch = () => modal.remove();
  document.getElementById('close-match-modal').onclick = closeMatch;
  document.getElementById('cancel-match').onclick = closeMatch;

  const performSearch = async () => {
    const query = document.getElementById('match-search-query').value.trim();
    if (!query) {
      showToast('Search query cannot be empty', 'error');
      return;
    }

    const container = document.getElementById('match-results-container');
    container.innerHTML = `
      <div class="flex items-center justify-center py-8">
        <div class="animate-spin rounded-full h-8 w-8 border-b-2 border-accent"></div>
      </div>
    `;

    try {
      const results = await request('GET', `/api/search/authors?name=${encodeURIComponent(query)}`);
      
      if (!results || results.length === 0) {
        container.innerHTML = '<p class="text-black-100 text-xs py-4 text-center">No results found.</p>';
        return;
      }

      container.innerHTML = '';
      results.forEach(res => {
        const item = document.createElement('div');
        item.className = 'bg-black-500 border border-black-400 rounded p-4 flex items-start justify-between space-x-4 hover:border-black-300 transition-colors';
        
        const avatar = res.image ? `<img src="${escapeHtml(res.image)}" class="w-12 h-12 rounded-full object-cover bg-black-400 flex-shrink-0" alt="avatar">` : `<div class="w-12 h-12 rounded-full bg-black-400 flex items-center justify-center text-white font-bold flex-shrink-0 text-sm">${escapeHtml(res.name.charAt(0))}</div>`;
        
        item.innerHTML = `
          <div class="flex items-start space-x-3 flex-grow min-w-0">
            ${avatar}
            <div class="min-w-0 flex-grow">
              <h4 class="text-white text-xs font-bold truncate">${escapeHtml(res.name)}</h4>
              <p class="text-black-100 text-[10px] uppercase font-semibold">ASIN: ${escapeHtml(res.asin)}</p>
              ${res.description ? `<p class="text-black-100 text-xs mt-1 line-clamp-2 leading-relaxed">${escapeHtml(res.description)}</p>` : ''}
            </div>
          </div>
          <button class="select-match-btn bg-accent hover:bg-accent-hover text-black px-3 py-1.5 rounded text-[11px] font-bold flex-shrink-0">Select Match</button>
        `;

        item.querySelector('.select-match-btn').onclick = async () => {
          try {
            item.querySelector('.select-match-btn').disabled = true;
            item.querySelector('.select-match-btn').innerText = 'Matching...';
            
            await request('POST', `/api/authors/${author.id}/match`, {
              asin: res.asin,
              provider: 'audnexus'
            });

            showToast('Author successfully matched!', 'success');
            closeMatch();
            if (parentCloseModal) parentCloseModal();
            loadAuthorDetails(author.id);
          } catch (err) {
            showToast('Failed to match author: ' + err.message, 'error');
            item.querySelector('.select-match-btn').disabled = false;
            item.querySelector('.select-match-btn').innerText = 'Select Match';
          }
        };

        container.appendChild(item);
      });
    } catch (err) {
      container.innerHTML = `<p class="text-red-500 text-xs py-4 text-center">Failed to fetch search results: ${escapeHtml(err.message)}</p>`;
    }
  };

  document.getElementById('match-search-btn').onclick = performSearch;
  document.getElementById('match-search-query').addEventListener('keyup', (e) => {
    if (e.key === 'Enter') {
      performSearch();
    }
  });

  // Perform initial search automatically
  performSearch();
}


function openEditSeriesModal(series) {
  const modal = document.createElement('div');
  modal.className = 'fixed inset-0 bg-black-900/80 z-50 flex items-center justify-center p-4';
  
  modal.innerHTML = `
    <div class="bg-black-600 border border-black-400 rounded-lg max-w-lg w-full flex flex-col overflow-hidden max-h-[90vh] text-left">
      <div class="px-6 py-4 border-b border-black-400 flex items-center justify-between">
        <h3 class="text-lg font-bold text-white">Edit Series</h3>
        <button id="close-series-modal" class="text-black-100 hover:text-white font-bold text-xl">&times;</button>
      </div>
      <div class="p-6 overflow-y-auto space-y-4">
        <div>
          <label class="block text-xs uppercase font-semibold text-black-100 mb-1.5">Name</label>
          <input type="text" id="edit-series-name" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent text-xs" value="${escapeHtml(series.name)}">
        </div>
        <div>
          <label class="block text-xs uppercase font-semibold text-black-100 mb-1.5">Name Ignore Prefix</label>
          <input type="text" id="edit-series-ignoreprefix" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent text-xs" value="${escapeHtml(series.nameIgnorePrefix || '')}">
        </div>
        <div>
          <label class="block text-xs uppercase font-semibold text-black-100 mb-1.5">Description</label>
          <textarea id="edit-series-description" rows="5" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent text-xs">${escapeHtml(series.description || '')}</textarea>
        </div>
      </div>
      <div class="px-6 py-4 border-t border-black-400 flex justify-end space-x-3">
        <button id="cancel-series-edit" class="bg-black-400 hover:bg-black-300 text-white px-4 py-2 rounded text-xs font-semibold">Cancel</button>
        <button id="save-series-edit" class="bg-accent hover:bg-accent-hover text-black px-4 py-2 rounded text-xs font-semibold">Save</button>
      </div>
    </div>
  `;

  document.body.appendChild(modal);

  const closeModal = () => modal.remove();
  document.getElementById('close-series-modal').onclick = closeModal;
  document.getElementById('cancel-series-edit').onclick = closeModal;

  document.getElementById('save-series-edit').onclick = async () => {
    const name = document.getElementById('edit-series-name').value.trim();
    const nameIgnorePrefix = document.getElementById('edit-series-ignoreprefix').value.trim();
    const description = document.getElementById('edit-series-description').value.trim();

    if (!name) {
      showToast('Name is required', 'error');
      return;
    }

    try {
      await request('PATCH', `/api/series/${series.id}`, {
        name,
        nameIgnorePrefix,
        description
      });
      showToast('Series updated successfully', 'success');
      closeModal();
      loadSeriesDetails(series.id);
    } catch (err) {
      showToast('Failed to update series: ' + err.message, 'error');
    }
  };
}
