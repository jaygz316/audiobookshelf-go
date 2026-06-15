import { request, resolvePath } from './api.js';
import { playItem } from './player.js';
import { openEbookReader } from './reader.js';

let currentUser = null;

async function getCurrentUser() {
  if (currentUser) return currentUser;
  try {
    currentUser = await request('GET', '/api/me');
    return currentUser;
  } catch (err) {
    console.error('Failed to retrieve user details:', err);
    return null;
  }
}

/**
 * Loads the details for a specific library item and renders them.
 * @param {string} itemId - The ID of the item to load.
 * @param {string} libraryId - The active library ID.
 * @param {function} backCallback - Function to execute when clicking "Back".
 */
export async function loadItemDetails(itemId, libraryId, backCallback) {
  const container = document.getElementById('bookshelf');
  if (!container) return;

  container.innerHTML = `
    <div class="flex items-center justify-center h-full mt-20">
      <div class="animate-spin rounded-full h-12 w-12 border-b-2 border-accent"></div>
    </div>
  `;

  try {
    const item = await request('GET', `/api/items/${itemId}`);
    const user = await getCurrentUser();
    const isAdmin = user && (user.type === 'root' || user.type === 'admin');

    const token = localStorage.getItem('token');
    const ts = item.updatedAt || item.addedAt || Date.now();
    const coverUrl = resolvePath(`/api/items/${item.id}/cover?token=${token}&ts=${ts}`);

    let title = 'Untitled';
    let subtitle = '';
    let authorName = 'Unknown';
    let narratorName = '';
    let seriesName = '';
    let seriesSequence = '';
    let description = '';
    let publisher = '';
    let publishedYear = '';
    let publishedDate = '';
    let isbn = '';
    let asin = '';
    let language = '';
    let explicit = false;
    let abridged = false;
    let tags = [];
    let genres = [];
    let mediaType = item.mediaType || 'book';

    if (item.media) {
      const metadata = item.media.metadata || {};
      title = metadata.title || item.title || title;
      subtitle = metadata.subtitle || '';
      authorName = metadata.authorName || metadata.author || authorName;
      narratorName = metadata.narratorName || (metadata.narrators ? metadata.narrators.join(', ') : '');
      description = metadata.description || '';
      publisher = metadata.publisher || '';
      publishedYear = metadata.publishedYear || '';
      publishedDate = metadata.publishedDate || '';
      isbn = metadata.isbn || '';
      asin = metadata.asin || '';
      language = metadata.language || '';
      explicit = !!metadata.explicit;
      abridged = !!metadata.abridged;
      tags = item.media.tags || [];
      genres = metadata.genres || [];

      if (metadata.series && metadata.series.length > 0) {
        seriesName = metadata.series[0].name || '';
        seriesSequence = metadata.series[0].sequence || '';
      } else if (metadata.seriesName) {
        seriesName = metadata.seriesName;
      }
    }

    const hasAudio = item.media && (
      item.media.duration > 0 || 
      (item.media.audioFiles && item.media.audioFiles.length > 0) ||
      (item.media.tracks && item.media.tracks.length > 0) ||
      (item.media.episodes && item.media.episodes.length > 0)
    );

    const hasEbook = item.media && (item.media.ebookFile || item.media.ebookFormat);

    // Update Toolbar details
    const viewTitle = document.getElementById('view-title');
    if (viewTitle) viewTitle.textContent = title;
    const bookCount = document.getElementById('book-count');
    if (bookCount) {
      bookCount.textContent = mediaType === 'podcast' ? 'Podcast' : 'Book';
    }

    container.innerHTML = `
      <div class="p-6 space-y-6 max-w-5xl mx-auto">
        <!-- Navigation Header -->
        <div class="flex items-center justify-between border-b border-black-600/50 pb-4">
          <button id="details-back-btn" class="flex items-center space-x-1.5 text-sm text-black-50 hover:text-white transition-colors">
            <span class="material-symbols">arrow_back</span>
            <span>Back</span>
          </button>
          ${isAdmin ? `
            <div class="flex items-center space-x-2">
              <button id="details-match-btn" class="bg-black-500 hover:bg-black-400 border border-black-300 text-white font-semibold px-3 py-1.5 rounded text-xs flex items-center space-x-1 transition-colors">
                <span class="material-symbols text-sm">find_replace</span>
                <span>Match</span>
              </button>
              <button id="details-edit-btn" class="bg-black-500 hover:bg-black-400 border border-black-300 text-white font-semibold px-3 py-1.5 rounded text-xs flex items-center space-x-1 transition-colors">
                <span class="material-symbols text-sm">edit</span>
                <span>Edit Details</span>
              </button>
            </div>
          ` : ''}
        </div>

        <!-- Main Grid Layout -->
        <div class="grid grid-cols-1 md:grid-cols-3 gap-8">
          <!-- Left Column: Cover & Core Actions -->
          <div class="flex flex-col items-center space-y-4">
            <div class="w-56 h-80 bg-black-500 rounded border border-black-400 overflow-hidden shadow-2xl flex-shrink-0 flex items-center justify-center relative group select-none">
              <img src="${coverUrl}" alt="${escapeHtml(title)}" class="w-full h-full object-cover" id="item-details-cover-img">
            </div>
            
            <!-- Core Play/Read Buttons -->
            <div class="w-full space-y-2 max-w-xs">
              ${hasAudio ? `
                <button id="details-play-action-btn" class="w-full bg-accent hover:opacity-90 text-primary font-bold py-2.5 px-4 rounded-md transition-all flex items-center justify-center space-x-2 text-sm shadow hover:scale-[1.02] duration-200">
                  <span class="material-symbols text-lg font-bold">play_arrow</span>
                  <span>Play Audiobook</span>
                </button>
              ` : ''}
              
              ${hasEbook ? `
                <button id="details-read-action-btn" class="w-full bg-black-500 hover:bg-black-400 border border-black-300 text-white font-bold py-2.5 px-4 rounded-md transition-all flex items-center justify-center space-x-2 text-sm shadow hover:scale-[1.02] duration-200">
                  <span class="material-symbols text-lg font-bold">menu_book</span>
                  <span>Read Book</span>
                </button>
              ` : ''}
              
              ${isAdmin ? `
                <button id="details-match-cover-btn" class="w-full bg-black-500 hover:bg-black-400 border border-black-300 text-white font-semibold py-2 px-4 rounded-md transition-all flex items-center justify-center space-x-2 text-xs shadow hover:scale-[1.02] duration-200">
                  <span class="material-symbols text-sm">image</span>
                  <span>${item.media?.coverPath ? 'Change Cover' : 'Get Cover Art'}</span>
                </button>
              ` : ''}
            </div>
          </div>

          <!-- Middle & Right Columns: Metadata & Info -->
          <div class="md:col-span-2 space-y-6 text-left">
            <div>
              <h2 class="text-3xl font-bold text-white tracking-wide">${escapeHtml(title)}</h2>
              ${subtitle ? `<p class="text-base text-black-50 mt-1">${escapeHtml(subtitle)}</p>` : ''}
            </div>

            <!-- Author & Series Line -->
            <div class="flex flex-wrap gap-x-6 gap-y-2 text-sm text-black-100">
              <div class="flex items-center space-x-1">
                <span class="material-symbols text-base text-accent">person</span>
                <span class="font-medium text-white">${escapeHtml(authorName)}</span>
              </div>
              ${seriesName ? `
                <div class="flex items-center space-x-1">
                  <span class="material-symbols text-base text-accent">layers</span>
                  <span>Series: <span class="font-medium text-white">${escapeHtml(seriesName)}</span> ${seriesSequence ? `(Book ${seriesSequence})` : ''}</span>
                </div>
              ` : ''}
              ${narratorName ? `
                <div class="flex items-center space-x-1">
                  <span class="material-symbols text-base text-accent">record_voice_over</span>
                  <span>Narrator: <span class="font-medium text-white">${escapeHtml(narratorName)}</span></span>
                </div>
              ` : ''}
            </div>

            <!-- Description -->
            ${description ? `
              <div class="bg-primary border border-black-300 rounded-md p-4 space-y-2">
                <h3 class="text-xs font-semibold uppercase tracking-wider text-black-100">Description</h3>
                <p class="text-sm text-black-50 leading-relaxed whitespace-pre-line overflow-y-auto max-h-48 no-scroll">${escapeHtml(description)}</p>
              </div>
            ` : ''}

            <!-- Metadata Grid -->
            <div class="grid grid-cols-2 md:grid-cols-3 gap-4 text-xs bg-primary/40 border border-black-400/50 rounded-md p-4">
              ${publisher ? `
                <div>
                  <p class="text-black-100 uppercase font-semibold">Publisher</p>
                  <p class="text-white mt-0.5 text-sm">${escapeHtml(publisher)}</p>
                </div>
              ` : ''}
              ${publishedYear || publishedDate ? `
                <div>
                  <p class="text-black-100 uppercase font-semibold">Published</p>
                  <p class="text-white mt-0.5 text-sm">${escapeHtml(publishedDate || publishedYear)}</p>
                </div>
              ` : ''}
              ${language ? `
                <div>
                  <p class="text-black-100 uppercase font-semibold">Language</p>
                  <p class="text-white mt-0.5 text-sm">${escapeHtml(language)}</p>
                </div>
              ` : ''}
              ${isbn ? `
                <div>
                  <p class="text-black-100 uppercase font-semibold">ISBN</p>
                  <p class="text-white mt-0.5 text-sm">${escapeHtml(isbn)}</p>
                </div>
              ` : ''}
              ${asin ? `
                <div>
                  <p class="text-black-100 uppercase font-semibold">ASIN</p>
                  <p class="text-white mt-0.5 text-sm">${escapeHtml(asin)}</p>
                </div>
              ` : ''}
              ${item.size ? `
                <div>
                  <p class="text-black-100 uppercase font-semibold">Size</p>
                  <p class="text-white mt-0.5 text-sm">${formatBytes(item.size)}</p>
                </div>
              ` : ''}
              ${item.media && item.media.duration ? `
                <div>
                  <p class="text-black-100 uppercase font-semibold">Duration</p>
                  <p class="text-white mt-0.5 text-sm">${formatDuration(item.media.duration)}</p>
                </div>
              ` : ''}
              <div>
                <p class="text-black-100 uppercase font-semibold">Explicit</p>
                <p class="text-white mt-0.5 text-sm">${explicit ? 'Yes' : 'No'}</p>
              </div>
              ${mediaType === 'book' ? `
                <div>
                  <p class="text-black-100 uppercase font-semibold">Abridged</p>
                  <p class="text-white mt-0.5 text-sm">${abridged ? 'Yes' : 'No'}</p>
                </div>
              ` : ''}
            </div>

            <!-- Genres & Tags -->
            <div class="space-y-3">
              ${genres.length > 0 ? `
                <div class="space-y-1">
                  <h4 class="text-xs uppercase font-semibold text-black-100">Genres</h4>
                  <div class="flex flex-wrap gap-2">
                    ${genres.map(g => `<span class="bg-black-500 border border-black-300 text-black-50 px-2.5 py-0.5 rounded-full text-xs font-medium">${escapeHtml(g)}</span>`).join('')}
                  </div>
                </div>
              ` : ''}
              
              ${tags.length > 0 ? `
                <div class="space-y-1">
                  <h4 class="text-xs uppercase font-semibold text-black-100">Tags</h4>
                  <div class="flex flex-wrap gap-2">
                    ${tags.map(t => `<span class="bg-accent/10 border border-accent/20 text-accent px-2.5 py-0.5 rounded-full text-xs font-medium">${escapeHtml(t)}</span>`).join('')}
                  </div>
                </div>
              ` : ''}
            </div>

            <!-- Tracks / Episode Accordion -->
            ${mediaType === 'podcast' && item.media && item.media.episodes && item.media.episodes.length > 0 ? `
              <div class="space-y-2">
                <h3 class="font-bold text-sm text-white border-b border-black-400 pb-1">Episodes (${item.media.episodes.length})</h3>
                <ul class="space-y-2 max-h-64 overflow-y-auto no-scroll border border-black-400/50 rounded-md p-2 bg-primary/20">
                  ${item.media.episodes.map((ep, idx) => `
                    <li class="flex items-center justify-between p-2 hover:bg-black-500/40 rounded transition-colors text-xs">
                      <div class="truncate flex-grow mr-4">
                        <p class="font-semibold text-white truncate">${escapeHtml(ep.title)}</p>
                        ${ep.pubDate ? `<p class="text-[0.7rem] text-black-100 mt-0.5">${escapeHtml(ep.pubDate)}</p>` : ''}
                      </div>
                      <button class="podcast-ep-play-btn flex items-center space-x-1 bg-accent text-primary px-2.5 py-1 rounded font-bold hover:opacity-90" data-idx="${idx}">
                        <span class="material-symbols text-sm font-bold">play_arrow</span>
                        <span>Play</span>
                      </button>
                    </li>
                  `).join('')}
                </ul>
              </div>
            ` : ''}

            ${mediaType === 'book' && item.media && item.media.tracks && item.media.tracks.length > 0 ? `
              <div class="space-y-2">
                <h3 class="font-bold text-sm text-white border-b border-black-400 pb-1">Audio Tracks (${item.media.tracks.length})</h3>
                <ol class="space-y-1 max-h-64 overflow-y-auto no-scroll border border-black-400/50 rounded-md p-2 bg-primary/20 list-decimal list-inside text-xs">
                  ${item.media.tracks.map((t, idx) => `
                    <li class="p-2 hover:bg-black-500/40 rounded transition-colors text-black-50">
                      <span class="font-medium text-white pl-1">${escapeHtml(t.title)}</span>
                      <span class="float-right text-[0.7rem] text-black-100">${formatDuration(t.duration)}</span>
                    </li>
                  `).join('')}
                </ol>
              </div>
            ` : ''}
          </div>
        </div>
      </div>
    `;

    // Hook click events
    document.getElementById('details-back-btn').onclick = backCallback;

    if (isAdmin) {
      const matchBtn = document.getElementById('details-match-btn');
      if (matchBtn) {
        matchBtn.onclick = () => triggerMatchBookModal(item, libraryId, () => loadItemDetails(itemId, libraryId, backCallback));
      }
      const matchCoverBtn = document.getElementById('details-match-cover-btn');
      if (matchCoverBtn) {
        matchCoverBtn.onclick = () => triggerMatchCoverModal(item, libraryId, () => loadItemDetails(itemId, libraryId, backCallback));
      }
      document.getElementById('details-edit-btn').onclick = () => triggerEditItemDetailsModal(item, libraryId, () => loadItemDetails(itemId, libraryId, backCallback));
    }

    const coverImg = document.getElementById('item-details-cover-img');
    if (coverImg) {
      coverImg.addEventListener('error', function() {
        this.src = 'assets/images/logo.png';
      }, { once: true });
    }

    if (hasAudio) {
      const playActionBtn = document.getElementById('details-play-action-btn');
      if (playActionBtn) {
        playActionBtn.onclick = async () => {
          try {
            let startTime = 0;
            try {
              const progressObj = await request('GET', `/api/me/progress/${item.id}`);
              if (progressObj && progressObj.currentTime !== undefined) {
                startTime = progressObj.currentTime;
              }
            } catch (err) {
              // Ignore progress fetch error, starts from 0
            }
            playItem(item, startTime);
          } catch (err) {
            console.error('Failed to start item playback:', err);
          }
        };
      }

      // Hook podcast episode clicks
      const epPlayBtns = container.querySelectorAll('.podcast-ep-play-btn');
      epPlayBtns.forEach(btn => {
        btn.onclick = () => {
          const idx = parseInt(btn.getAttribute('data-idx'), 10);
          const episode = item.media.episodes[idx];
          // We can construct a mock item or play the episode
          const mockItem = {
            ...item,
            media: {
              ...item.media,
              audioFiles: [episode.audioFile],
              duration: episode.duration || 0,
              metadata: {
                ...item.media.metadata,
                title: episode.title
              }
            }
          };
          playItem(mockItem, 0);
        };
      });
    }

    if (hasEbook) {
      const readActionBtn = document.getElementById('details-read-action-btn');
      if (readActionBtn) {
        readActionBtn.onclick = () => {
          openEbookReader(item, token);
        };
      }
    }

  } catch (err) {
    console.error('Failed to load item details:', err);
    container.innerHTML = `
      <div class="p-6 text-center text-red-400">
        <span class="material-symbols text-4xl mb-2">error</span>
        <p class="text-sm font-medium">Failed to load item details: ${escapeHtml(err.message)}</p>
        <button id="details-error-back-btn" class="mt-4 bg-black-500 hover:bg-black-400 text-white px-4 py-2 rounded text-xs">Go Back</button>
      </div>
    `;
    const errBackBtn = document.getElementById('details-error-back-btn');
    if (errBackBtn) errBackBtn.onclick = backCallback;
  }
}

/**
 * Triggers a beautiful Modal to edit the item details.
 */
function triggerEditItemDetailsModal(item, libraryId, onSaveSuccess) {
  const mediaType = item.mediaType || 'book';
  let title = '';
  let subtitle = '';
  let authors = [];
  let narrators = [];
  let seriesName = '';
  let seriesSequence = '';
  let publisher = '';
  let publishedYear = '';
  let publishedDate = '';
  let description = '';
  let isbn = '';
  let asin = '';
  let language = '';
  let explicit = false;
  let abridged = false;
  let tags = item.media?.tags || [];
  let genres = [];

  if (item.media) {
    const metadata = item.media.metadata || {};
    title = metadata.title || item.title || '';
    subtitle = metadata.subtitle || '';
    
    if (mediaType === 'book') {
      if (metadata.authors) {
        authors = metadata.authors.map(a => a.name || a);
      } else if (metadata.authorName) {
        authors = [metadata.authorName];
      }
    } else if (mediaType === 'podcast') {
      if (metadata.author) {
        authors = [metadata.author];
      }
    }

    narrators = metadata.narrators || [];
    description = metadata.description || '';
    publisher = metadata.publisher || '';
    publishedYear = metadata.publishedYear || '';
    publishedDate = metadata.publishedDate || '';
    isbn = metadata.isbn || '';
    asin = metadata.asin || '';
    language = metadata.language || '';
    explicit = !!metadata.explicit;
    abridged = !!metadata.abridged;
    genres = metadata.genres || [];

    if (metadata.series && metadata.series.length > 0) {
      seriesName = metadata.series[0].name || '';
      seriesSequence = metadata.series[0].sequence || '';
    } else if (metadata.seriesName) {
      seriesName = metadata.seriesName;
    }
  }

  const modal = document.createElement('div');
  modal.className = 'fixed inset-0 bg-black-900/80 z-50 flex items-center justify-center p-4 select-none';
  modal.innerHTML = `
    <div class="bg-primary border border-black-300 w-full max-w-2xl p-6 rounded-md shadow-2xl space-y-4 flex flex-col max-h-[90vh]">
      <!-- Header -->
      <div class="flex justify-between items-center border-b border-black-400 pb-2 flex-shrink-0">
        <h3 class="text-lg font-bold text-white flex items-center space-x-2">
          <span class="material-symbols text-accent">edit_note</span>
          <span>Edit Item Details</span>
        </h3>
        <button id="close-edit-item-modal" class="text-black-100 hover:text-white transition-colors">
          <span class="material-symbols text-xl">close</span>
        </button>
      </div>
      
      <!-- Scrollable Edit Form -->
      <form id="edit-item-form" class="space-y-4 overflow-y-auto no-scroll pr-1 flex-grow">
        <!-- Title & Subtitle -->
        <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
          <div>
            <label class="block text-[0.7rem] uppercase font-semibold text-black-100 mb-1.5">Title</label>
            <input type="text" id="edit-item-title" required value="${escapeHtml(title)}" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent text-xs">
          </div>
          <div>
            <label class="block text-[0.7rem] uppercase font-semibold text-black-100 mb-1.5">Subtitle</label>
            <input type="text" id="edit-item-subtitle" value="${escapeHtml(subtitle)}" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent text-xs">
          </div>
        </div>

        <!-- Authors & Narrators -->
        <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
          <div>
            <label class="block text-[0.7rem] uppercase font-semibold text-black-100 mb-1.5">${mediaType === 'podcast' ? 'Author / Host' : 'Author(s) (comma separated)'}</label>
            <input type="text" id="edit-item-authors" value="${escapeHtml(authors.join(', '))}" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent text-xs">
          </div>
          ${mediaType === 'book' ? `
            <div>
              <label class="block text-[0.7rem] uppercase font-semibold text-black-100 mb-1.5">Narrator(s) (comma separated)</label>
              <input type="text" id="edit-item-narrators" value="${escapeHtml(narrators.join(', '))}" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent text-xs">
            </div>
          ` : ''}
        </div>

        <!-- Series (Only Book) -->
        ${mediaType === 'book' ? `
          <div class="grid grid-cols-3 gap-4">
            <div class="col-span-2">
              <label class="block text-[0.7rem] uppercase font-semibold text-black-100 mb-1.5">Series Name</label>
              <input type="text" id="edit-item-series" value="${escapeHtml(seriesName)}" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent text-xs">
            </div>
            <div>
              <label class="block text-[0.7rem] uppercase font-semibold text-black-100 mb-1.5">Sequence</label>
              <input type="text" id="edit-item-sequence" value="${escapeHtml(seriesSequence)}" placeholder="e.g. 1, 1.5" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent text-xs">
            </div>
          </div>
        ` : ''}

        <!-- Description -->
        <div>
          <label class="block text-[0.7rem] uppercase font-semibold text-black-100 mb-1.5">Description</label>
          <textarea id="edit-item-description" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent text-xs h-24 resize-none">${escapeHtml(description)}</textarea>
        </div>

        <!-- Publisher & Dates -->
        <div class="grid grid-cols-3 gap-4">
          <div>
            <label class="block text-[0.7rem] uppercase font-semibold text-black-100 mb-1.5">Publisher</label>
            <input type="text" id="edit-item-publisher" value="${escapeHtml(publisher)}" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent text-xs">
          </div>
          <div>
            <label class="block text-[0.7rem] uppercase font-semibold text-black-100 mb-1.5">Publish Year</label>
            <input type="text" id="edit-item-pubyear" value="${escapeHtml(publishedYear)}" placeholder="e.g. 2023" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent text-xs">
          </div>
          <div>
            <label class="block text-[0.7rem] uppercase font-semibold text-black-100 mb-1.5">Publish Date</label>
            <input type="text" id="edit-item-pubdate" value="${escapeHtml(publishedDate)}" placeholder="YYYY-MM-DD" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent text-xs">
          </div>
        </div>

        <!-- ISBN, ASIN, Language -->
        <div class="grid grid-cols-3 gap-4">
          <div>
            <label class="block text-[0.7rem] uppercase font-semibold text-black-100 mb-1.5">ISBN</label>
            <input type="text" id="edit-item-isbn" value="${escapeHtml(isbn)}" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent text-xs">
          </div>
          <div>
            <label class="block text-[0.7rem] uppercase font-semibold text-black-100 mb-1.5">ASIN</label>
            <input type="text" id="edit-item-asin" value="${escapeHtml(asin)}" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent text-xs">
          </div>
          <div>
            <label class="block text-[0.7rem] uppercase font-semibold text-black-100 mb-1.5">Language</label>
            <input type="text" id="edit-item-language" value="${escapeHtml(language)}" placeholder="e.g. English" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent text-xs">
          </div>
        </div>

        <!-- Explicit / Abridged Checkboxes -->
        <div class="flex items-center space-x-6 py-2 border-t border-b border-black-400/50">
          <label class="flex items-center space-x-2 text-xs font-semibold text-white cursor-pointer">
            <input type="checkbox" id="edit-item-explicit" ${explicit ? 'checked' : ''} class="w-4 h-4 rounded text-accent bg-black-600 border-black-300 focus:ring-accent">
            <span>Explicit Content</span>
          </label>
          ${mediaType === 'book' ? `
            <label class="flex items-center space-x-2 text-xs font-semibold text-white cursor-pointer">
              <input type="checkbox" id="edit-item-abridged" ${abridged ? 'checked' : ''} class="w-4 h-4 rounded text-accent bg-black-600 border-black-300 focus:ring-accent">
              <span>Abridged Book</span>
            </label>
          ` : ''}
        </div>

        <!-- Tags & Genres -->
        <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
          <div>
            <label class="block text-[0.7rem] uppercase font-semibold text-black-100 mb-1.5">Genres (comma separated)</label>
            <input type="text" id="edit-item-genres" value="${escapeHtml(genres.join(', '))}" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent text-xs">
          </div>
          <div>
            <label class="block text-[0.7rem] uppercase font-semibold text-black-100 mb-1.5">Tags (comma separated)</label>
            <input type="text" id="edit-item-tags" value="${escapeHtml(tags.join(', '))}" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent text-xs">
          </div>
        </div>
      </form>

      <!-- Footer Buttons -->
      <div class="flex justify-end space-x-3 pt-2 border-t border-black-400 flex-shrink-0">
        <button id="cancel-edit-item-btn" class="bg-black-400 hover:bg-black-300 text-white font-semibold px-4 py-2 rounded text-xs transition-colors">Cancel</button>
        <button id="save-edit-item-btn" class="bg-accent hover:opacity-90 text-primary font-bold px-4 py-2 rounded text-xs transition-opacity shadow">Save Changes</button>
      </div>
    </div>
  `;

  document.body.appendChild(modal);

  const closeModal = () => modal.remove();

  document.getElementById('close-edit-item-modal').onclick = closeModal;
  document.getElementById('cancel-edit-item-btn').onclick = closeModal;

  document.getElementById('save-edit-item-btn').onclick = async (e) => {
    e.preventDefault();

    const titleVal = document.getElementById('edit-item-title').value.trim();
    if (!titleVal) {
      alert('Title is required');
      return;
    }

    const payload = {
      title: titleVal,
      subtitle: document.getElementById('edit-item-subtitle').value.trim(),
      authors: splitCommaList(document.getElementById('edit-item-authors').value),
      narrators: mediaType === 'book' ? splitCommaList(document.getElementById('edit-item-narrators').value) : [],
      seriesName: mediaType === 'book' ? document.getElementById('edit-item-series').value.trim() : '',
      seriesSequence: mediaType === 'book' ? document.getElementById('edit-item-sequence').value.trim() : '',
      description: document.getElementById('edit-item-description').value.trim(),
      publisher: document.getElementById('edit-item-publisher').value.trim(),
      publishedYear: document.getElementById('edit-item-pubyear').value.trim(),
      publishedDate: document.getElementById('edit-item-pubdate').value.trim(),
      isbn: document.getElementById('edit-item-isbn').value.trim(),
      asin: document.getElementById('edit-item-asin').value.trim(),
      language: document.getElementById('edit-item-language').value.trim(),
      explicit: document.getElementById('edit-item-explicit').checked,
      abridged: mediaType === 'book' ? document.getElementById('edit-item-abridged').checked : false,
      genres: splitCommaList(document.getElementById('edit-item-genres').value),
      tags: splitCommaList(document.getElementById('edit-item-tags').value)
    };

    try {
      await request('PATCH', `/api/items/${item.id}`, payload);
      closeModal();
      if (typeof onSaveSuccess === 'function') {
        onSaveSuccess();
      }
    } catch (err) {
      alert('Failed to save changes: ' + err.message);
    }
  };
}

/**
 * Split helper
 */
function splitCommaList(str) {
  if (!str) return [];
  return str.split(',')
    .map(val => val.trim())
    .filter(val => val.length > 0);
}

/**
 * Format bytes helper
 */
function formatBytes(bytes) {
  if (!bytes) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
}

/**
 * Format duration helper
 */
function formatDuration(seconds) {
  if (!seconds) return '0:00';
  const h = Math.floor(seconds / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  const s = Math.floor(seconds % 60);

  const mStr = m < 10 && h > 0 ? '0' + m : m;
  const sStr = s < 10 ? '0' + s : s;

  if (h > 0) {
    return `${h}:${mStr}:${sStr}`;
  }
  return `${mStr}:${sStr}`;
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

function triggerMatchModal(item, libraryId, mode, onSaveSuccess) {
  const isCoverMode = mode === 'cover';
  const mediaType = item.mediaType || 'book';
  const currentTitle = item.media?.metadata?.title || item.title || '';
  let currentAuthor = '';
  if (mediaType === 'book') {
    if (item.media?.metadata?.authors) {
      currentAuthor = item.media.metadata.authors.map(a => a.name || a).join(', ');
    } else if (item.media?.metadata?.authorName) {
      currentAuthor = item.media.metadata.authorName;
    }
  } else if (mediaType === 'podcast') {
    currentAuthor = item.media?.metadata?.author || '';
  }

  // Create Modal
  const modal = document.createElement('div');
  modal.className = 'fixed inset-0 bg-black-900/80 backdrop-blur-sm z-50 flex items-center justify-center p-4 select-none';
  modal.innerHTML = `
    <div class="bg-primary border border-black-300 w-full max-w-2xl p-6 rounded-md shadow-2xl space-y-4 flex flex-col max-h-[90vh]">
      <!-- Header -->
      <div class="flex justify-between items-center border-b border-black-400 pb-2 flex-shrink-0">
        <h3 class="text-lg font-bold text-white flex items-center space-x-2">
          <span class="material-symbols text-accent">${isCoverMode ? 'image' : 'find_replace'}</span>
          <span>${isCoverMode ? 'Get Cover Art' : 'Match Book Details'}</span>
        </h3>
        <button id="close-match-modal" class="text-black-100 hover:text-white transition-colors">
          <span class="material-symbols text-xl">close</span>
        </button>
      </div>

      <!-- Search Section -->
      <div class="grid grid-cols-1 sm:grid-cols-4 gap-3 flex-shrink-0">
        <div class="sm:col-span-1">
          <label class="block text-[0.7rem] uppercase font-semibold text-black-100 mb-1">Provider</label>
          <select id="match-provider-select" class="w-full bg-black-500 text-white px-2 py-1.5 rounded border border-black-300 focus:outline-none focus:border-accent text-xs">
            <!-- Dynamically populated -->
          </select>
        </div>
        <div class="sm:col-span-2">
          <label class="block text-[0.7rem] uppercase font-semibold text-black-100 mb-1">Title</label>
          <input type="text" id="match-title-input" value="${escapeHtml(currentTitle)}" class="w-full bg-black-500 text-white px-2 py-1.5 rounded border border-black-300 focus:outline-none focus:border-accent text-xs">
        </div>
        <div class="sm:col-span-1">
          <label class="block text-[0.7rem] uppercase font-semibold text-black-100 mb-1">Author</label>
          <input type="text" id="match-author-input" value="${escapeHtml(currentAuthor)}" class="w-full bg-black-500 text-white px-2 py-1.5 rounded border border-black-300 focus:outline-none focus:border-accent text-xs">
        </div>
      </div>

      <div class="flex-shrink-0 flex justify-end">
        <button id="match-search-btn" class="bg-accent hover:opacity-90 text-primary font-bold px-4 py-1.5 rounded text-xs transition-opacity shadow flex items-center space-x-1">
          <span class="material-symbols text-sm">search</span>
          <span>Search</span>
        </button>
      </div>

      <!-- Results & Preview Section -->
      <div id="match-results-container" class="flex-grow overflow-y-auto border border-black-300/30 rounded-md p-3 bg-black-500/20 space-y-3 min-h-[150px] max-h-[30vh] no-scroll">
        <p class="text-xs text-black-100 text-center py-6">Enter search criteria and click Search.</p>
      </div>

      <!-- Selected Result Details (Import Options) -->
      <div id="match-details-container" class="flex-shrink-0 border-t border-black-400 pt-3 hidden space-y-3">
        <h4 class="text-xs uppercase font-semibold text-white">Choose fields to import:</h4>
        <div id="match-fields-checkboxes" class="grid grid-cols-2 sm:grid-cols-3 gap-2 p-3 bg-black-500/30 border border-black-300/30 rounded-md">
          <!-- Dynamically populated checkboxes -->
        </div>
      </div>

      <!-- Footer Buttons -->
      <div class="flex justify-end space-x-3 pt-2 border-t border-black-400 flex-shrink-0">
        <button id="cancel-match-btn" class="bg-black-400 hover:bg-black-300 text-white font-semibold px-4 py-2 rounded text-xs transition-colors">Cancel</button>
        <button id="import-match-btn" class="bg-accent hover:opacity-90 text-primary font-bold px-4 py-2 rounded text-xs transition-opacity shadow hidden">Import Selected</button>
      </div>
    </div>
  `;

  document.body.appendChild(modal);

  const closeModal = () => modal.remove();
  document.getElementById('close-match-modal').onclick = closeModal;
  document.getElementById('cancel-match-btn').onclick = closeModal;

  const providerSelect = document.getElementById('match-provider-select');
  const resultsContainer = document.getElementById('match-results-container');
  const detailsContainer = document.getElementById('match-details-container');
  const checkboxesContainer = document.getElementById('match-fields-checkboxes');
  const importBtn = document.getElementById('import-match-btn');
  let selectedResult = null;

  // Fetch Providers
  request('GET', '/api/search/providers')
    .then(data => {
      const providers = isCoverMode ? (data.providers?.booksCovers || []) : (data.providers?.books || []);
      providerSelect.innerHTML = providers.map(p => `<option value="${p.value}">${escapeHtml(p.text)}</option>`).join('');
      // Set default provider (Google Books is a good default, or fallback to first)
      if (providers.some(p => p.value === 'google')) {
        providerSelect.value = 'google';
      }
    })
    .catch(err => {
      console.error('Failed to load search providers:', err);
      providerSelect.innerHTML = `<option value="google">Google Books</option><option value="openlibrary">Open Library</option>`;
    });

  // Search Action
  const performSearch = async () => {
    const provider = providerSelect.value;
    const title = document.getElementById('match-title-input').value.trim();
    const author = document.getElementById('match-author-input').value.trim();

    resultsContainer.innerHTML = `
      <div class="flex items-center justify-center py-8">
        <div class="animate-spin rounded-full h-8 w-8 border-b-2 border-accent"></div>
      </div>
    `;
    detailsContainer.classList.add('hidden');
    importBtn.classList.add('hidden');
    selectedResult = null;

    try {
      let results = [];
      if (mediaType === 'book') {
        const queryParams = new URLSearchParams({ provider, title, author });
        results = await request('GET', `/api/search/books?${queryParams.toString()}`);
      } else {
        // Podcast search
        const queryParams = new URLSearchParams({ term: title || author });
        results = await request('GET', `/api/search/podcast?${queryParams.toString()}`);
      }

      if (!results || results.length === 0) {
        resultsContainer.innerHTML = `<p class="text-xs text-black-100 text-center py-6">No results found.</p>`;
        return;
      }

      renderSearchResults(results);
    } catch (err) {
      console.error('Search failed:', err);
      resultsContainer.innerHTML = `<p class="text-xs text-red-400 text-center py-6">Search failed: ${escapeHtml(err.message)}</p>`;
    }
  };

  document.getElementById('match-search-btn').onclick = performSearch;

  // Render search results
  function renderSearchResults(results) {
    resultsContainer.innerHTML = results.map((res, idx) => {
      const coverHtml = res.coverUrl 
        ? `<img src="${escapeHtml(res.coverUrl)}" class="w-12 h-18 bg-black-500 rounded border border-black-400 object-cover flex-shrink-0" alt="">`
        : `<div class="w-12 h-18 bg-black-500 rounded border border-black-400 flex items-center justify-center flex-shrink-0"><span class="material-symbols text-xl text-black-200">image</span></div>`;
      
      const authorText = res.authors && res.authors.length > 0 ? res.authors.join(', ') : 'Unknown Author';
      const yearText = res.publishedYear ? `(${res.publishedYear})` : '';
      const publisherText = res.publisher ? ` - ${res.publisher}` : '';

      return `
        <div class="match-result-item flex items-start space-x-3 p-2.5 rounded border border-black-400/50 hover:bg-black-500/50 cursor-pointer transition-colors" data-idx="${idx}">
          ${coverHtml}
          <div class="flex-grow min-w-0 text-left text-xs">
            <p class="font-bold text-white truncate text-sm">${escapeHtml(res.title)}</p>
            ${res.subtitle ? `<p class="text-black-100 truncate mt-0.5">${escapeHtml(res.subtitle)}</p>` : ''}
            <p class="text-black-50 mt-1">${escapeHtml(authorText)}</p>
            <p class="text-black-100 mt-1">${escapeHtml(yearText)}${escapeHtml(publisherText)}</p>
          </div>
        </div>
      `;
    }).join('');

    // Hook selection click
    const resultItems = resultsContainer.querySelectorAll('.match-result-item');
    resultItems.forEach(itemEl => {
      itemEl.onclick = () => {
        resultItems.forEach(el => el.classList.remove('bg-accent/10', 'border-accent'));
        itemEl.classList.add('bg-accent/10', 'border-accent');
        
        const idx = parseInt(itemEl.getAttribute('data-idx'), 10);
        selectedResult = results[idx];
        
        if (isCoverMode) {
          // In cover-only mode, directly show save button
          importBtn.classList.remove('hidden');
          importBtn.textContent = 'Import Cover Art';
        } else {
          // Full metadata match mode: show checklist
          showImportCheckboxes(selectedResult);
        }
      };
    });
  }

  // Populate checkboxes
  function showImportCheckboxes(res) {
    detailsContainer.classList.remove('hidden');
    importBtn.classList.remove('hidden');
    importBtn.textContent = 'Import Selected';

    const fields = [
      { key: 'title', label: 'Title', value: res.title },
      { key: 'subtitle', label: 'Subtitle', value: res.subtitle },
      { key: 'authors', label: 'Authors', value: res.authors?.join(', ') },
      { key: 'narrators', label: 'Narrators', value: res.narrators?.join(', ') },
      { key: 'publisher', label: 'Publisher', value: res.publisher },
      { key: 'publishedYear', label: 'Published Year', value: res.publishedYear },
      { key: 'description', label: 'Description', value: res.description },
      { key: 'isbn', label: 'ISBN', value: res.isbn },
      { key: 'asin', label: 'ASIN', value: res.asin },
      { key: 'language', label: 'Language', value: res.language },
      { key: 'cover', label: 'Cover Image', value: res.coverUrl }
    ];

    checkboxesContainer.innerHTML = fields
      .filter(f => f.value && String(f.value).trim().length > 0)
      .map(f => `
        <label class="flex items-center space-x-2 text-xs text-black-50 cursor-pointer hover:text-white transition-colors py-1 truncate" title="${escapeHtml(f.label)}: ${escapeHtml(f.value)}">
          <input type="checkbox" id="match-cb-${f.key}" checked class="match-import-cb w-4 h-4 rounded text-accent bg-black-600 border-black-300 focus:ring-accent">
          <span class="truncate">${escapeHtml(f.label)}</span>
        </label>
      `).join('');
  }

  // Submit Handler
  importBtn.onclick = async (e) => {
    e.preventDefault();
    if (!selectedResult) return;

    importBtn.disabled = true;
    importBtn.innerHTML = `
      <div class="animate-spin rounded-full h-3 w-3 border-b-2 border-primary mr-1 inline-block"></div>
      <span>Importing...</span>
    `;

    try {
      if (isCoverMode) {
        // Cover-only Mode
        if (!selectedResult.coverUrl) {
          alert('Selected item does not have cover art.');
          importBtn.disabled = false;
          importBtn.textContent = 'Import Cover Art';
          return;
        }
        await request('POST', `/api/items/${item.id}/cover-from-url`, { coverUrl: selectedResult.coverUrl });
      } else {
        // Full Metadata Match Mode
        const cbChecked = (key) => {
          const el = document.getElementById(`match-cb-${key}`);
          return el ? el.checked : false;
        };

        // If cover is checked, download it first
        if (cbChecked('cover') && selectedResult.coverUrl) {
          try {
            await request('POST', `/api/items/${item.id}/cover-from-url`, { coverUrl: selectedResult.coverUrl });
          } catch (coverErr) {
            console.error('Failed to import cover art:', coverErr);
            // We can show a warning and continue with metadata import
          }
        }

        // Construct PATCH payload (merging matching result with existing data)
        const payload = {
          title: cbChecked('title') ? selectedResult.title : currentTitle,
          subtitle: cbChecked('subtitle') ? selectedResult.subtitle : (item.media?.metadata?.subtitle || ''),
          authors: cbChecked('authors') ? selectedResult.authors : (item.media?.metadata?.authors?.map(a => a.name || a) || (item.media?.metadata?.authorName ? [item.media.metadata.authorName] : [])),
          narrators: cbChecked('narrators') ? selectedResult.narrators : (item.media?.metadata?.narrators || []),
          seriesName: item.media?.metadata?.series?.[0]?.name || item.media?.metadata?.seriesName || '',
          seriesSequence: item.media?.metadata?.series?.[0]?.sequence || '',
          description: cbChecked('description') ? selectedResult.description : (item.media?.metadata?.description || ''),
          publisher: cbChecked('publisher') ? selectedResult.publisher : (item.media?.metadata?.publisher || ''),
          publishedYear: cbChecked('publishedYear') ? selectedResult.publishedYear : (item.media?.metadata?.publishedYear || ''),
          publishedDate: item.media?.metadata?.publishedDate || '',
          isbn: cbChecked('isbn') ? selectedResult.isbn : (item.media?.metadata?.isbn || ''),
          asin: cbChecked('asin') ? selectedResult.asin : (item.media?.metadata?.asin || ''),
          language: cbChecked('language') ? selectedResult.language : (item.media?.metadata?.language || ''),
          explicit: !!item.media?.metadata?.explicit,
          abridged: !!item.media?.metadata?.abridged,
          genres: item.media?.metadata?.genres || [],
          tags: item.media?.tags || []
        };

        await request('PATCH', `/api/items/${item.id}`, payload);
      }

      closeModal();
      if (typeof onSaveSuccess === 'function') {
        onSaveSuccess();
      }
    } catch (err) {
      console.error('Import failed:', err);
      alert('Import failed: ' + err.message);
      importBtn.disabled = false;
      importBtn.textContent = isCoverMode ? 'Import Cover Art' : 'Import Selected';
    }
  };
}

function triggerMatchBookModal(item, libraryId, onSaveSuccess) {
  triggerMatchModal(item, libraryId, 'metadata', onSaveSuccess);
}

function triggerMatchCoverModal(item, libraryId, onSaveSuccess) {
  triggerMatchModal(item, libraryId, 'cover', onSaveSuccess);
}
