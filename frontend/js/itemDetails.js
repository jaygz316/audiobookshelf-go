import { request, resolvePath } from './api.js';
import { playItem, getCurrentPlayingItem, getCurrentPlaybackTime } from './player.js';
import { openEbookReader } from './reader.js';

let currentUser = null;
let activeItemId = null;
let activeLibraryId = null;
let activeBackCallback = null;
let currentBookmarkListener = null;

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
  activeItemId = itemId;
  activeLibraryId = libraryId;
  activeBackCallback = backCallback;

  const opmlBtn = document.getElementById('opml-btn');
  if (opmlBtn) opmlBtn.classList.add('hidden');

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
              <img src="${coverUrl}" alt="${escapeHtml(title)}" class="w-full h-full object-cover" onerror="this.onerror=null; this.src='assets/images/logo.png'">
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
                <button id="details-send-device-btn" class="w-full bg-black-500 hover:bg-black-400 border border-black-300 text-white font-semibold py-2 px-4 rounded-md transition-all flex items-center justify-center space-x-2 text-xs shadow hover:scale-[1.02] duration-200 mt-2">
                  <span class="material-symbols text-sm">send_to_mobile</span>
                  <span>Send to Device</span>
                </button>
              ` : ''}
              
              ${isAdmin ? `
                <button id="details-match-cover-btn" class="w-full bg-black-500 hover:bg-black-400 border border-black-300 text-white font-semibold py-2 px-4 rounded-md transition-all flex items-center justify-center space-x-2 text-xs shadow hover:scale-[1.02] duration-200">
                  <span class="material-symbols text-sm">image</span>
                  <span>${item.media?.coverPath ? 'Change Cover' : 'Get Cover Art'}</span>
                </button>
                <button id="details-embed-metadata-btn" class="w-full bg-black-500 hover:bg-black-400 border border-black-300 text-white font-semibold py-2 px-4 rounded-md transition-all flex items-center justify-center space-x-2 text-xs shadow hover:scale-[1.02] duration-200 mt-2">
                  <span class="material-symbols text-sm">settings_suggest</span>
                  <span>Embed Metadata</span>
                </button>
                ${item.media?.audioFiles && item.media.audioFiles.length > 1 ? `
                  <button id="details-merge-audio-btn" class="w-full bg-black-500 hover:bg-black-400 border border-black-300 text-white font-semibold py-2 px-4 rounded-md transition-all flex items-center justify-center space-x-2 text-xs shadow hover:scale-[1.02] duration-200 mt-2">
                    <span class="material-symbols text-sm">call_merge</span>
                    <span>Merge Audio Files</span>
                  </button>
              ` : ''}
              <button id="details-share-btn" class="w-full bg-black-500 hover:bg-black-400 border border-black-300 text-white font-semibold py-2 px-4 rounded-md transition-all flex items-center justify-center space-x-2 text-xs shadow hover:scale-[1.02] duration-200 mt-2">
                <span class="material-symbols text-sm">share</span>
                <span>Share Link</span>
              </button>
            </div>

            <!-- RSS Feed Status & Management Section -->
            <div id="details-rss-section" class="w-full max-w-xs border border-black-400/50 bg-primary/20 rounded-md p-3.5 space-y-3 text-xs text-left">
              <div class="flex items-center justify-between border-b border-black-500 pb-2">
                <span class="font-bold text-white uppercase tracking-wider">RSS Feed</span>
                <span id="rss-status-badge" class="px-2 py-0.5 rounded text-[0.65rem] font-bold uppercase tracking-wide bg-black-500 text-black-100">Closed</span>
              </div>
              <div id="rss-controls" class="space-y-2">
                <div class="animate-spin rounded-full h-4 w-4 border-b-2 border-accent mx-auto"></div>
              </div>
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
                ${(item.media?.metadata?.authors && item.media.metadata.authors.length > 0) ?
                  item.media.metadata.authors.map((auth, idx) => `
                    <span class="author-link font-medium text-white hover:text-accent cursor-pointer transition-colors hover:underline" data-id="${auth.id}" data-name="${escapeHtml(auth.name)}">${escapeHtml(auth.name)}</span>${idx < item.media.metadata.authors.length - 1 ? '<span class="text-black-100">, </span>' : ''}
                  `).join('') : `<span class="font-medium text-white">${escapeHtml(authorName)}</span>`
                }
              </div>
              ${(item.media?.metadata?.series && item.media.metadata.series.length > 0) ?
                item.media.metadata.series.map(ser => `
                  <div class="flex items-center space-x-1">
                    <span class="material-symbols text-base text-accent">layers</span>
                    <span>Series: <span class="series-link font-medium text-white hover:text-accent cursor-pointer transition-colors hover:underline" data-id="${ser.id}" data-name="${escapeHtml(ser.name)}">${escapeHtml(ser.name)}</span> ${ser.sequence ? `(Book ${ser.sequence})` : ''}</span>
                  </div>
                `).join('') : (seriesName ? `
                  <div class="flex items-center space-x-1">
                    <span class="material-symbols text-base text-accent">layers</span>
                    <span>Series: <span class="font-medium text-white">${escapeHtml(seriesName)}</span> ${seriesSequence ? `(Book ${seriesSequence})` : ''}</span>
                  </div>
                ` : '')
              }
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

            ${mediaType === 'book' && hasAudio ? `
              <div class="space-y-2">
                <div class="flex items-center justify-between border-b border-black-400 pb-1">
                  <h3 class="font-bold text-sm text-white flex items-center space-x-1">
                    <span class="material-symbols text-sm text-accent">toc</span>
                    <span>Chapters (${item.media?.chapters?.length || 0})</span>
                  </h3>
                  ${isAdmin ? `
                    <button id="details-edit-chapters-btn" class="text-xs text-accent hover:underline flex items-center space-x-1">
                      <span class="material-symbols text-sm">edit</span>
                      <span>Edit Chapters</span>
                    </button>
                  ` : ''}
                </div>
                ${item.media?.chapters?.length > 0 ? `
                  <ol class="space-y-1 max-h-64 overflow-y-auto no-scroll border border-black-400/50 rounded-md p-2 bg-primary/20 list-decimal list-inside text-xs">
                    ${item.media.chapters.map((c) => `
                      <li class="p-2 hover:bg-black-500/40 rounded transition-colors text-black-50 flex justify-between items-center">
                        <span class="font-medium text-white pl-1">${escapeHtml(c.title)}</span>
                        <span class="float-right text-[0.7rem] text-black-100">${formatDuration(c.start)} - ${formatDuration(c.end)}</span>
                      </li>
                    `).join('')}
                  </ol>
                ` : `
                  <p class="text-xs text-black-100">No chapters defined. Click "Edit Chapters" to create or lookup chapters.</p>
                `}
              </div>
            ` : ''}

            <!-- Bookmarks Section -->
            ${hasAudio ? `
              <div id="details-bookmarks-section" class="space-y-2 mt-4">
                <div class="flex items-center justify-between border-b border-black-400 pb-1">
                  <h3 class="font-bold text-sm text-white flex items-center space-x-1">
                    <span class="material-symbols text-sm text-accent">bookmarks</span>
                    <span>Bookmarks</span>
                  </h3>
                  <div class="flex items-center space-x-3">
                    <button id="details-add-bookmark-btn" class="text-xs text-accent hover:underline flex items-center space-x-1">
                      <span class="material-symbols text-sm">bookmark_add</span>
                      <span>Add Bookmark</span>
                    </button>
                    <button id="details-export-bookmarks-btn" class="text-xs text-accent hover:underline flex items-center space-x-1">
                      <span class="material-symbols text-sm">download</span>
                      <span>Export</span>
                    </button>
                  </div>
                </div>
                <div id="bookmarks-list-container"></div>
              </div>
            ` : ''}

            <!-- Other Versions / Narrators Section -->
            ${item.otherVersions && item.otherVersions.length > 0 ? `
              <div id="details-other-versions-section" class="space-y-2 mt-4 text-left">
                <div class="flex items-center justify-between border-b border-black-400 pb-1">
                  <h3 class="font-bold text-sm text-white flex items-center space-x-1">
                    <span class="material-symbols text-sm text-accent font-bold">library_books</span>
                    <span>Other Versions / Narrators (${item.otherVersions.length})</span>
                  </h3>
                </div>
                <div class="grid grid-cols-1 sm:grid-cols-2 gap-3 pt-2">
                  ${item.otherVersions.map(v => {
                    const token = localStorage.getItem('token');
                    const ts = Date.now();
                    const vCoverUrl = v.coverPath 
                      ? resolvePath(`/api/items/${v.id}/cover?token=${token}&ts=${ts}`)
                      : 'assets/images/logo.png';
                    const narratorsStr = v.narrators && v.narrators.length > 0 
                      ? v.narrators.join(', ') 
                      : 'Unknown Narrator';
                    const durationStr = formatDuration(v.duration);
                    return `
                      <div class="other-version-card flex items-center space-x-3 p-2.5 rounded-md border border-black-400/50 bg-primary/10 hover:bg-black-500/40 cursor-pointer transition-all duration-200" data-id="${v.id}">
                        <img src="${vCoverUrl}" class="w-10 h-14 object-cover rounded border border-black-400/60 flex-shrink-0" onerror="this.onerror=null; this.src='assets/images/logo.png'">
                        <div class="min-w-0 flex-grow text-xs text-left">
                          <p class="font-bold text-white truncate text-sm">${escapeHtml(v.title)}</p>
                          ${v.subtitle ? `<p class="text-black-100 truncate mt-0.5">${escapeHtml(v.subtitle)}</p>` : ''}
                          <p class="text-black-50 mt-1 flex items-center space-x-1">
                            <span class="material-symbols text-[11px] text-accent">record_voice_over</span>
                            <span class="truncate">${escapeHtml(narratorsStr)}</span>
                          </p>
                          <p class="text-black-100 mt-0.5 flex items-center space-x-1">
                            <span class="material-symbols text-[11px] text-black-100">schedule</span>
                            <span>${durationStr}</span>
                          </p>
                        </div>
                      </div>
                    `;
                  }).join('')}
                </div>
              </div>
            ` : ''}
          </div>
        </div>
      </div>
    `;

    // Hook click events
    document.getElementById('details-back-btn').onclick = backCallback;

    container.querySelectorAll('.other-version-card').forEach(card => {
      card.onclick = () => {
        loadItemDetails(card.dataset.id, libraryId, backCallback);
      };
    });

    container.querySelectorAll('.author-link').forEach(link => {
      link.onclick = (e) => {
        e.preventDefault();
        window.dispatchEvent(new CustomEvent('navigate-to-author', {
          detail: {
            authorId: link.dataset.id,
            authorName: link.dataset.name
          }
        }));
      };
    });

    container.querySelectorAll('.series-link').forEach(link => {
      link.onclick = (e) => {
        e.preventDefault();
        window.dispatchEvent(new CustomEvent('navigate-to-series', {
          detail: {
            seriesId: link.dataset.id,
            seriesName: link.dataset.name
          }
        }));
      };
    });

    const shareBtn = document.getElementById('details-share-btn');
    if (shareBtn) {
      shareBtn.onclick = () => triggerShareLinkModal(item);
    }

    if (isAdmin) {
      const matchBtn = document.getElementById('details-match-btn');
      if (matchBtn) {
        matchBtn.onclick = () => triggerMatchBookModal(item, libraryId, () => loadItemDetails(itemId, libraryId, backCallback));
      }
      const matchCoverBtn = document.getElementById('details-match-cover-btn');
      if (matchCoverBtn) {
        matchCoverBtn.onclick = () => triggerMatchCoverModal(item, libraryId, () => loadItemDetails(itemId, libraryId, backCallback));
      }
      const embedMetadataBtn = document.getElementById('details-embed-metadata-btn');
      if (embedMetadataBtn) {
        embedMetadataBtn.onclick = async () => {
          if (!confirm('Are you sure you want to write and embed metadata, chapters, and cover art directly into the audio files? This will overwrite the tags of the files on disk.')) {
            return;
          }
          embedMetadataBtn.disabled = true;
          embedMetadataBtn.innerHTML = '<span class="material-symbols text-sm animate-spin">sync</span><span>Embedding...</span>';
          try {
            const resp = await request('POST', `/api/items/${item.id}/embed-metadata`);
            alert(resp.message || 'Metadata embedded successfully!');
          } catch (err) {
            alert('Failed to embed metadata: ' + (err.message || err));
          } finally {
            embedMetadataBtn.disabled = false;
            embedMetadataBtn.innerHTML = '<span class="material-symbols text-sm">settings_suggest</span><span>Embed Metadata</span>';
          }
        };
      }
      const mergeAudioBtn = document.getElementById('details-merge-audio-btn');
      if (mergeAudioBtn) {
        mergeAudioBtn.onclick = async () => {
          if (!confirm('Are you sure you want to merge all separate audio tracks into a single M4B file? This will merge the files, create chapters, update the database, and delete the original files.')) {
            return;
          }
          mergeAudioBtn.disabled = true;
          mergeAudioBtn.innerHTML = '<span class="material-symbols text-sm animate-spin">sync</span><span>Merging...</span>';
          try {
            const resp = await request('POST', `/api/items/${item.id}/merge`);
            alert(resp.message || 'Audio files merged successfully!');
            loadItemDetails(itemId, libraryId, backCallback);
          } catch (err) {
            alert('Failed to merge audio files: ' + (err.message || err));
          } finally {
            mergeAudioBtn.disabled = false;
            mergeAudioBtn.innerHTML = '<span class="material-symbols text-sm">call_merge</span><span>Merge Audio Files</span>';
          }
        };
      }
      document.getElementById('details-edit-btn').onclick = () => triggerEditItemDetailsModal(item, libraryId, () => loadItemDetails(itemId, libraryId, backCallback));
      const editChaptersBtn = document.getElementById('details-edit-chapters-btn');
      if (editChaptersBtn) {
        editChaptersBtn.onclick = () => triggerEditChaptersModal(item, () => loadItemDetails(itemId, libraryId, backCallback));
      }
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

      const sendDeviceBtn = document.getElementById('details-send-device-btn');
      if (sendDeviceBtn) {
        sendDeviceBtn.onclick = async () => {
          try {
            // Fetch available devices
            const devices = await request('GET', '/api/emails/devices');
            if (!devices || devices.length === 0) {
              alert('No e-reader devices are currently configured or available for your account.');
              return;
            }

            // Create a selection modal/popup
            const modal = document.createElement('div');
            modal.className = 'fixed inset-0 bg-black-900/80 z-50 flex items-center justify-center p-4';
            modal.innerHTML = `
              <div class="bg-primary border border-black-300 w-full max-w-sm rounded-md shadow-2xl p-6 relative text-left">
                <h3 class="text-md font-bold text-white mb-3 border-b border-black-400 pb-2">Send E-Book to Device</h3>
                
                <div class="space-y-4">
                  <div>
                    <label class="block text-xs font-semibold text-black-100 uppercase tracking-wider mb-2">Select Target Device</label>
                    <select id="send-target-device" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent">
                      ${devices.map(dev => `<option value="${escapeHtml(dev.name)}">${escapeHtml(dev.name)} (${escapeHtml(dev.email)})</option>`).join('')}
                    </select>
                  </div>

                  <div class="flex justify-end space-x-3 pt-2">
                    <button type="button" id="close-send-modal-btn" class="bg-black-400 hover:bg-black-300 text-white px-4 py-2 rounded text-xs font-semibold">Cancel</button>
                    <button type="button" id="confirm-send-device-btn" class="bg-accent hover:opacity-90 text-primary font-bold px-4 py-2 rounded text-xs">Send</button>
                  </div>
                </div>
              </div>
            `;

            document.body.appendChild(modal);

            const closeModal = () => modal.remove();
            modal.querySelector('#close-send-modal-btn').onclick = closeModal;

            modal.querySelector('#confirm-send-device-btn').onclick = async () => {
              const selectedDevice = modal.querySelector('#send-target-device').value;
              const sendBtn = modal.querySelector('#confirm-send-device-btn');
              sendBtn.textContent = 'Sending...';
              sendBtn.disabled = true;

              try {
                await request('POST', '/api/emails/send-ebook-to-device', {
                  libraryItemId: item.id,
                  deviceName: selectedDevice
                });
                alert(`E-Book successfully queued to send to "${selectedDevice}"!`);
                closeModal();
              } catch (err) {
                alert('Failed to send e-book: ' + err.message);
                sendBtn.textContent = 'Send';
                sendBtn.disabled = false;
              }
            };

          } catch (err) {
            alert('Error loading devices: ' + err.message);
          }
        };
      }
    }

    if (hasAudio) {
      // Load bookmarks
      renderBookmarks(item);

      // Hook up add bookmark button
      const addBookmarkBtn = document.getElementById('details-add-bookmark-btn');
      if (addBookmarkBtn) {
        addBookmarkBtn.onclick = () => {
          triggerAddBookmarkOnDetailsModal(item);
        };
      }

      // Hook up export bookmarks button
      const exportBookmarksBtn = document.getElementById('details-export-bookmarks-btn');
      if (exportBookmarksBtn) {
        exportBookmarksBtn.onclick = () => {
          triggerExportBookmarksModal(item);
        };
      }

      // Hook up global bookmark-added listener for updates
      const onBookmarkUpdate = (e) => {
        if (e.detail && e.detail.itemId === item.id) {
          renderBookmarks(item);
        }
      };

      if (currentBookmarkListener) {
        window.removeEventListener('bookmark-added', currentBookmarkListener);
      }
      window.addEventListener('bookmark-added', onBookmarkUpdate);
      currentBookmarkListener = onBookmarkUpdate;
    } else {
      // Clear listener if we navigate to an item without audio
      if (currentBookmarkListener) {
        window.removeEventListener('bookmark-added', currentBookmarkListener);
        currentBookmarkListener = null;
      }
    }

    // RSS Feed Rendering & Logic
    const rssStatusBadge = document.getElementById('rss-status-badge');
    const rssControls = document.getElementById('rss-controls');
    
    async function updateRssSection() {
      if (!rssStatusBadge || !rssControls) return;
      try {
        const feedsResp = await request('GET', '/api/feeds');
        const feeds = feedsResp.feeds || [];
        const activeFeed = feeds.find(f => f.entityId === item.id);
        
        if (activeFeed) {
          rssStatusBadge.className = 'px-2 py-0.5 rounded text-[0.65rem] font-bold uppercase tracking-wide bg-accent/20 text-accent';
          rssStatusBadge.textContent = 'Active';
          
          rssControls.innerHTML = `
            <div class="space-y-1.5">
              <label class="text-[0.65rem] text-black-100 font-semibold uppercase">Feed URL</label>
              <div class="flex gap-1.5">
                <input type="text" id="rss-feed-url-input" readonly value="${escapeHtml(activeFeed.feedUrl)}" class="flex-grow bg-black-500 text-white font-mono text-[0.7rem] px-2.5 py-1.5 rounded border border-black-300 focus:outline-none select-all">
                <button id="rss-copy-btn" class="bg-accent hover:opacity-90 text-primary font-bold px-3 rounded transition-colors text-[0.7rem]">
                  Copy
                </button>
              </div>
            </div>
            ${isAdmin ? `
              <button id="rss-action-btn" class="w-full bg-error hover:opacity-90 text-white font-bold py-1.5 px-3 rounded transition-all text-[0.7rem]">
                Close RSS Feed
              </button>
            ` : ''}
          `;
          
          const copyBtn = document.getElementById('rss-copy-btn');
          if (copyBtn) {
            copyBtn.onclick = () => {
              const urlInput = document.getElementById('rss-feed-url-input');
              navigator.clipboard.writeText(urlInput ? urlInput.value : activeFeed.feedUrl).then(() => {
                const oldText = copyBtn.textContent;
                copyBtn.textContent = 'Copied';
                setTimeout(() => { copyBtn.textContent = oldText; }, 2000);
              });
            };
          }
          
          const actionBtn = document.getElementById('rss-action-btn');
          if (actionBtn) {
            actionBtn.onclick = async () => {
              if (!confirm('Are you sure you want to close this RSS feed?')) return;
              try {
                await request('DELETE', `/api/feeds/${activeFeed.id}`);
                updateRssSection();
              } catch (err) {
                alert('Failed to close RSS feed: ' + err.message);
              }
            };
          }
        } else {
          rssStatusBadge.className = 'px-2 py-0.5 rounded text-[0.65rem] font-bold uppercase tracking-wide bg-black-500 text-black-100';
          rssStatusBadge.textContent = 'Closed';
          
          if (isAdmin) {
            rssControls.innerHTML = `
              <p class="text-black-100 text-[0.7rem]">Generate a public RSS feed to subscribe to this item in external podcast players.</p>
              <button id="rss-action-btn" class="w-full bg-accent hover:opacity-90 text-primary font-bold py-1.5 px-3 rounded transition-all text-[0.7rem]">
                Open Public RSS Feed
              </button>
            `;
            
            const actionBtn = document.getElementById('rss-action-btn');
            if (actionBtn) {
              actionBtn.onclick = async () => {
                try {
                  await request('POST', '/api/feeds', {
                    entityId: item.id,
                    type: mediaType
                  });
                  updateRssSection();
                } catch (err) {
                  alert('Failed to open RSS feed: ' + err.message);
                }
              };
            }
          } else {
            rssControls.innerHTML = `
              <p class="text-black-100 text-[0.7rem]">No active RSS feed. Public RSS feeds must be enabled by an administrator.</p>
            `;
          }
        }
      } catch (err) {
        rssControls.innerHTML = `<p class="text-error text-[0.75rem]">Failed to query RSS details: ${escapeHtml(err.message)}</p>`;
      }
    }
    
    updateRssSection();

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

  // Deep Matching state trackers
  const fieldValues = {
    title: { label: 'Title', current: currentTitle, activeSource: 'current', options: { current: currentTitle } },
    subtitle: { label: 'Subtitle', current: (item.media?.metadata?.subtitle || ''), activeSource: 'current', options: { current: (item.media?.metadata?.subtitle || '') } },
    authors: { label: 'Authors', current: currentAuthor, activeSource: 'current', options: { current: currentAuthor }, type: 'array' },
    narrators: { label: 'Narrators', current: (item.media?.metadata?.narrators || []).join(', '), activeSource: 'current', options: { current: (item.media?.metadata?.narrators || []).join(', ') }, type: 'array' },
    publisher: { label: 'Publisher', current: (item.media?.metadata?.publisher || ''), activeSource: 'current', options: { current: (item.media?.metadata?.publisher || '') } },
    publishedYear: { label: 'Published Year', current: (item.media?.metadata?.publishedYear || ''), activeSource: 'current', options: { current: (item.media?.metadata?.publishedYear || '') } },
    description: { label: 'Description', current: (item.media?.metadata?.description || ''), activeSource: 'current', options: { current: (item.media?.metadata?.description || '') } },
    isbn: { label: 'ISBN', current: (item.media?.metadata?.isbn || ''), activeSource: 'current', options: { current: (item.media?.metadata?.isbn || '') } },
    asin: { label: 'ASIN', current: (item.media?.metadata?.asin || ''), activeSource: 'current', options: { current: (item.media?.metadata?.asin || '') } },
    language: { label: 'Language', current: (item.media?.metadata?.language || ''), activeSource: 'current', options: { current: (item.media?.metadata?.language || '') } },
    cover: { label: 'Cover Image', current: '', activeSource: 'current', options: { current: '' } }
  };

  const updateFieldOptions = (res, providerName) => {
    const fieldsToUpdate = {
      title: res.title,
      subtitle: res.subtitle,
      authors: res.authors?.join(', '),
      narrators: res.narrators?.join(', '),
      publisher: res.publisher,
      publishedYear: res.publishedYear,
      description: res.description,
      isbn: res.isbn,
      asin: res.asin,
      language: res.language,
      cover: res.coverUrl
    };

    for (const [key, val] of Object.entries(fieldsToUpdate)) {
      if (val && String(val).trim().length > 0) {
        fieldValues[key].options[providerName] = val;
        fieldValues[key].activeSource = providerName;
      }
    }
  };

  // Create Modal
  const modal = document.createElement('div');
  modal.className = 'fixed inset-0 bg-black-900/80 backdrop-blur-sm z-50 flex items-center justify-center p-4 select-none';
  modal.innerHTML = `
    <div class="bg-primary border border-black-300 w-full max-w-2xl p-6 rounded-md shadow-2xl space-y-4 flex flex-col max-h-[90vh]">
      <!-- Header -->
      <div class="flex justify-between items-center border-b border-black-400 pb-2 flex-shrink-0">
        <h3 class="text-lg font-bold text-white flex items-center space-x-2">
          <span class="material-symbols text-accent">${isCoverMode ? 'image' : 'find_replace'}</span>
          <span>${isCoverMode ? 'Get Cover Art' : 'Deep Multi-Provider Match'}</span>
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
        <h4 class="text-xs uppercase font-semibold text-white">Granular Metadata Merging (Select source per field):</h4>
        <div id="match-fields-checkboxes" class="grid grid-cols-1 gap-2 p-3 bg-black-500/30 border border-black-300/30 rounded-md">
          <!-- Dynamically populated fields with source selector dropdowns -->
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
      itemEl.onclick = async () => {
        resultItems.forEach(el => el.classList.remove('bg-accent/10', 'border-accent'));
        itemEl.classList.add('bg-accent/10', 'border-accent');
        
        const idx = parseInt(itemEl.getAttribute('data-idx'), 10);
        selectedResult = results[idx];
        
        if (isCoverMode) {
          // In cover-only mode, directly perform the cover art import on click
          await executeCoverImport(selectedResult, itemEl);
        } else {
          // Full metadata match mode: show checklist
          const providerName = providerSelect.options[providerSelect.selectedIndex]?.text || providerSelect.value;
          updateFieldOptions(selectedResult, providerName);
          renderMergedFieldsTable();
        }
      };
    });
  }

  // Helper function to handle direct cover art import with spinner feedback
  async function executeCoverImport(res, itemEl) {
    if (!res.coverUrl) {
      alert('Selected item does not have cover art.');
      return;
    }

    // Disable all result items to prevent double clicks/clicks on other items
    const resultItems = resultsContainer.querySelectorAll('.match-result-item');
    resultItems.forEach(el => el.style.pointerEvents = 'none');

    // Show loading spinner/styling
    if (itemEl) {
      itemEl.classList.add('opacity-60');
      const imgEl = itemEl.querySelector('img');
      if (imgEl) {
        imgEl.style.filter = 'blur(1px)';
      }
    }

    // Disable action buttons
    const searchBtn = document.getElementById('match-search-btn');
    if (searchBtn) searchBtn.disabled = true;
    const cancelBtn = document.getElementById('cancel-match-btn');
    if (cancelBtn) cancelBtn.disabled = true;
    const closeBtn = document.getElementById('close-match-modal');
    if (closeBtn) closeBtn.disabled = true;
    
    importBtn.classList.remove('hidden');
    importBtn.disabled = true;
    importBtn.innerHTML = `
      <div class="animate-spin rounded-full h-3 w-3 border-b-2 border-primary mr-1 inline-block"></div>
      <span>Importing...</span>
    `;

    try {
      await request('POST', `/api/items/${item.id}/cover-from-url`, { coverUrl: res.coverUrl });
      closeModal();
      if (typeof onSaveSuccess === 'function') {
        onSaveSuccess();
      }
    } catch (err) {
      console.error('Import failed:', err);
      alert('Import failed: ' + err.message);

      // Re-enable everything
      resultItems.forEach(el => el.style.pointerEvents = 'auto');
      if (itemEl) {
        itemEl.classList.remove('opacity-60');
        const imgEl = itemEl.querySelector('img');
        if (imgEl) {
          imgEl.style.filter = 'none';
        }
      }
      if (searchBtn) searchBtn.disabled = false;
      if (cancelBtn) cancelBtn.disabled = false;
      if (closeBtn) closeBtn.disabled = false;
      importBtn.disabled = false;
      importBtn.textContent = 'Import Cover Art';
    }
  }

  // Populate checkboxes
  function renderMergedFieldsTable() {
    detailsContainer.classList.remove('hidden');
    importBtn.classList.remove('hidden');
    importBtn.textContent = 'Import Selected';

    checkboxesContainer.className = 'space-y-3 p-3 bg-black-500/30 border border-black-300/30 rounded-md max-h-[30vh] overflow-y-auto no-scroll';

    checkboxesContainer.innerHTML = Object.entries(fieldValues).map(([key, f]) => {
      const optionKeys = Object.keys(f.options).filter(optKey => {
        const val = f.options[optKey];
        return val !== undefined && val !== null && String(val).trim().length > 0;
      });

      // If only one option (current) and it's empty, we don't show it
      if (optionKeys.length <= 1 && (!f.current || String(f.current).trim().length === 0)) {
        return '';
      }

      const selectOptionsHtml = optionKeys.map(optKey => {
        const isSelected = f.activeSource === optKey ? 'selected' : '';
        const displaySource = optKey === 'current' ? 'Current Value' : optKey;
        return `<option value="${escapeHtml(optKey)}" ${isSelected}>${escapeHtml(displaySource)}</option>`;
      }).join('');

      const activeValue = f.options[f.activeSource] || '';
      let previewHtml = '';
      if (key === 'cover' && activeValue) {
        previewHtml = `<img src="${escapeHtml(activeValue)}" class="w-10 h-15 rounded object-cover border border-black-400 mt-1" alt="">`;
      } else if (key === 'description' && activeValue) {
        previewHtml = `<p class="text-[11px] text-black-100 mt-1 line-clamp-2" title="${escapeHtml(activeValue)}">${escapeHtml(activeValue)}</p>`;
      } else if (activeValue) {
        previewHtml = `<p class="text-[11px] text-black-100 mt-1 truncate">${escapeHtml(activeValue)}</p>`;
      }

      return `
        <div class="flex flex-col space-y-1 pb-2 border-b border-black-400/30 last:border-b-0 last:pb-0">
          <div class="flex items-center justify-between">
            <span class="text-xs font-semibold text-white">${escapeHtml(f.label)}</span>
            <select id="match-select-${key}" class="bg-black-600 text-white text-[11px] px-2 py-0.5 rounded border border-black-400 focus:outline-none focus:border-accent max-w-[150px]">
              ${selectOptionsHtml}
            </select>
          </div>
          <div id="match-preview-${key}">
            ${previewHtml}
          </div>
        </div>
      `;
    }).join('');

    // Attach change listeners to update activeSource and preview
    Object.keys(fieldValues).forEach(key => {
      const selectEl = document.getElementById(`match-select-${key}`);
      if (selectEl) {
        selectEl.onchange = () => {
          const newSource = selectEl.value;
          fieldValues[key].activeSource = newSource;
          
          // Update preview
          const previewEl = document.getElementById(`match-preview-${key}`);
          if (previewEl) {
            const activeValue = fieldValues[key].options[newSource] || '';
            if (key === 'cover' && activeValue) {
              previewEl.innerHTML = `<img src="${escapeHtml(activeValue)}" class="w-10 h-15 rounded object-cover border border-black-400 mt-1" alt="">`;
            } else if (key === 'description' && activeValue) {
              previewEl.innerHTML = `<p class="text-[11px] text-black-100 mt-1 line-clamp-2" title="${escapeHtml(activeValue)}">${escapeHtml(activeValue)}</p>`;
            } else if (activeValue) {
              previewEl.innerHTML = `<p class="text-[11px] text-black-100 mt-1 truncate">${escapeHtml(activeValue)}</p>`;
            } else {
              previewEl.innerHTML = '';
            }
          }
        };
      }
    });
  }

  // Submit Handler
  importBtn.onclick = async (e) => {
    e.preventDefault();

    if (isCoverMode) {
      if (!selectedResult) return;
      const selectedEl = resultsContainer.querySelector('.match-result-item.border-accent');
      await executeCoverImport(selectedResult, selectedEl);
    } else {
      importBtn.disabled = true;
      importBtn.innerHTML = `
        <div class="animate-spin rounded-full h-3 w-3 border-b-2 border-primary mr-1 inline-block"></div>
        <span>Importing...</span>
      `;

      try {
        // Helper to get selected value
        const getSelectedValue = (key) => {
          const f = fieldValues[key];
          return f.options[f.activeSource];
        };

        // If cover is checked and selected source is not current, download it first
        const selectedCover = getSelectedValue('cover');
        if (fieldValues.cover.activeSource !== 'current' && selectedCover) {
          try {
            await request('POST', `/api/items/${item.id}/cover-from-url`, { coverUrl: selectedCover });
          } catch (coverErr) {
            console.error('Failed to import cover art:', coverErr);
            // We can continue with metadata import
          }
        }

        // Helper to check if a field is modified (activeSource is not current)
        const getMergedVal = (key, defaultVal) => {
          const f = fieldValues[key];
          if (f.activeSource === 'current') return defaultVal;
          const val = f.options[f.activeSource];
          if (f.type === 'array') {
            return val ? val.split(',').map(s => s.trim()).filter(Boolean) : [];
          }
          return val;
        };

        // Construct PATCH payload (merging matching result with existing data)
        const payload = {
          title: getMergedVal('title', currentTitle),
          subtitle: getMergedVal('subtitle', item.media?.metadata?.subtitle || ''),
          authors: getMergedVal('authors', item.media?.metadata?.authors?.map(a => a.name || a) || (item.media?.metadata?.authorName ? [item.media.metadata.authorName] : [])),
          narrators: getMergedVal('narrators', item.media?.metadata?.narrators || []),
          seriesName: item.media?.metadata?.series?.[0]?.name || item.media?.metadata?.seriesName || '',
          seriesSequence: item.media?.metadata?.series?.[0]?.sequence || '',
          description: getMergedVal('description', item.media?.metadata?.description || ''),
          publisher: getMergedVal('publisher', item.media?.metadata?.publisher || ''),
          publishedYear: getMergedVal('publishedYear', item.media?.metadata?.publishedYear || ''),
          publishedDate: item.media?.metadata?.publishedDate || '',
          isbn: getMergedVal('isbn', item.media?.metadata?.isbn || ''),
          asin: getMergedVal('asin', item.media?.metadata?.asin || ''),
          language: getMergedVal('language', item.media?.metadata?.language || ''),
          explicit: !!item.media?.metadata?.explicit,
          abridged: !!item.media?.metadata?.abridged,
          genres: item.media?.metadata?.genres || [],
          tags: item.media?.tags || []
        };

        await request('PATCH', `/api/items/${item.id}`, payload);
        closeModal();
        if (typeof onSaveSuccess === 'function') {
          onSaveSuccess();
        }
      } catch (err) {
        console.error('Import failed:', err);
        alert('Import failed: ' + err.message);
        importBtn.disabled = false;
        importBtn.textContent = 'Import Selected';
      }
    }
  };
}

function triggerMatchBookModal(item, libraryId, onSaveSuccess) {
  triggerMatchModal(item, libraryId, 'metadata', onSaveSuccess);
}

function triggerMatchCoverModal(item, libraryId, onSaveSuccess) {
  triggerMatchModal(item, libraryId, 'cover', onSaveSuccess);
}

async function renderBookmarks(item) {
  const container = document.getElementById('bookmarks-list-container');
  if (!container) return;

  container.innerHTML = `
    <div class="flex items-center justify-center p-4">
      <div class="animate-spin rounded-full h-6 w-6 border-b-2 border-accent"></div>
    </div>
  `;

  try {
    currentUser = await request('GET', '/api/me');
    const bookmarks = (currentUser.bookmarks || []).filter(b => b.libraryItemId === item.id);
    
    bookmarks.sort((a, b) => a.time - b.time);

    if (bookmarks.length === 0) {
      container.innerHTML = `
        <p class="text-xs text-black-100 italic py-2">No bookmarks saved for this audiobook.</p>
      `;
      return;
    }

    container.innerHTML = `
      <ul class="space-y-1.5 border border-black-400/50 rounded-md p-2 bg-primary/20 max-h-60 overflow-y-auto no-scroll">
        ${bookmarks.map((b, idx) => `
          <li class="flex items-center justify-between p-2 hover:bg-black-500/40 rounded transition-colors text-xs" data-time="${b.time}">
            <div class="flex items-center space-x-2 truncate flex-grow cursor-pointer bookmark-jump-btn mr-4">
              <span class="material-symbols text-sm text-accent">bookmark</span>
              <div class="truncate">
                <p class="font-semibold text-white truncate">${escapeHtml(b.title)}</p>
                <p class="text-[0.7rem] text-black-100 mt-0.5">${formatDuration(b.time)}</p>
              </div>
            </div>
            <div class="flex items-center space-x-2 flex-shrink-0">
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
        if (confirm(`Are you sure you want to delete the bookmark "${b.title}"?`)) {
          try {
            await request('DELETE', `/api/me/item/${item.id}/bookmark/${b.time}`);
            renderBookmarks(item);
          } catch (err) {
            console.error('Failed to delete bookmark:', err);
            alert('Failed to delete bookmark.');
          }
        }
      };
    });

  } catch (err) {
    console.error('Failed to render bookmarks:', err);
    container.innerHTML = `<p class="text-xs text-red-500">Failed to load bookmarks.</p>`;
  }
}

function triggerEditBookmarkModal(item, bookmark) {
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
  titleInput.value = bookmark.title;
  titleInput.focus();
  titleInput.select();

  const closeModal = () => modal.remove();
  document.getElementById('close-edit-bookmark-modal').onclick = closeModal;
  document.getElementById('cancel-edit-bookmark-btn').onclick = closeModal;

  document.getElementById('save-edit-bookmark-btn').onclick = async () => {
    const titleVal = titleInput.value.trim();
    if (!titleVal) {
      alert("Title is required");
      return;
    }

    try {
      await request('PATCH', `/api/me/item/${item.id}/bookmark`, {
        time: bookmark.time,
        title: titleVal
      });
      closeModal();
      renderBookmarks(item);
    } catch (err) {
      console.error('Failed to update bookmark:', err);
      alert('Failed to update bookmark: ' + (err.message || 'Unknown error'));
    }
  };
}

function triggerAddBookmarkOnDetailsModal(item) {
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
      alert("Both time and title are required.");
      return;
    }

    let parsedTime = 0;
    if (rawTime.includes(':')) {
      const parts = rawTime.split(':').map(Number);
      if (parts.some(isNaN)) {
        alert("Invalid time format. Please use HH:MM:SS or simple seconds.");
        return;
      }
      if (parts.length === 3) {
        parsedTime = parts[0] * 3600 + parts[1] * 60 + parts[2];
      } else if (parts.length === 2) {
        parsedTime = parts[0] * 60 + parts[1];
      } else {
        alert("Invalid time format. Please use HH:MM:SS or simple seconds.");
        return;
      }
    } else {
      parsedTime = Number(rawTime);
      if (isNaN(parsedTime)) {
        alert("Invalid time format. Please use HH:MM:SS or simple seconds.");
        return;
      }
    }

    try {
      await request('POST', `/api/me/item/${item.id}/bookmark`, {
        time: parsedTime,
        title: titleVal
      });
      closeModal();
      renderBookmarks(item);
    } catch (err) {
      console.error('Failed to create bookmark:', err);
      alert('Failed to save bookmark: ' + (err.message || 'Unknown error'));
    }
  };
}

function triggerExportBookmarksModal(item) {
  const bookmarks = (currentUser.bookmarks || []).filter(b => b.libraryItemId === item.id);
  bookmarks.sort((a, b) => a.time - b.time);

  if (bookmarks.length === 0) {
    alert("No bookmarks to export.");
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
    let content = `"Time (seconds)","Timestamp","Title"\n`;
    bookmarks.forEach(b => {
      content += `"${b.time}","${formatDuration(b.time)}","${b.title.replace(/"/g, '""')}"\n`;
    });
    const filename = `${sanitizeName(item.title)}_bookmarks.csv`;
    downloadFile(filename, content, 'text/csv;charset=utf-8');
  };

  document.getElementById('export-json-btn').onclick = () => {
    const data = bookmarks.map(b => ({
      time: b.time,
      timestamp: formatDuration(b.time),
      title: b.title
    }));
    const content = JSON.stringify(data, null, 2);
    const filename = `${sanitizeName(item.title)}_bookmarks.json`;
    downloadFile(filename, content, 'application/json;charset=utf-8');
  };
}

/**
 * Parse duration format (HH:MM:SS or MM:SS or seconds) to float seconds
 */
function parseDuration(str) {
  if (!str) return 0;
  str = str.trim();
  if (/^\d+(\.\d+)?$/.test(str)) {
    return parseFloat(str);
  }
  const parts = str.split(':').map(Number);
  if (parts.some(isNaN)) return 0;
  if (parts.length === 3) {
    return parts[0] * 3600 + parts[1] * 60 + parts[2];
  } else if (parts.length === 2) {
    return parts[0] * 60 + parts[1];
  }
  return 0;
}

/**
 * Triggers interactive chapter editor modal
 */
function triggerEditChaptersModal(item, onSaveSuccess) {
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

/**
 * Triggers a Modal to configure or manage public share links.
 */
function triggerShareLinkModal(item) {
  const media = item.media || {};
  const title = media.title || item.title || 'Unknown Item';

  const modal = document.createElement('div');
  modal.className = 'fixed inset-0 bg-black-900/80 z-50 flex items-center justify-center p-4 select-none';
  modal.innerHTML = `
    <div class="bg-primary border border-black-300 w-full max-w-md p-6 rounded-md shadow-2xl space-y-4 flex flex-col">
      <!-- Header -->
      <div class="flex justify-between items-center border-b border-black-400 pb-2">
        <h3 class="text-lg font-bold text-white flex items-center space-x-2">
          <span class="material-symbols text-accent">share</span>
          <span>Share Link</span>
        </h3>
        <button id="close-share-modal" class="text-black-100 hover:text-white transition-colors">
          <span class="material-symbols text-xl">close</span>
        </button>
      </div>

      <div id="share-modal-body" class="space-y-4 text-sm text-left">
        <div class="flex justify-center py-4"><span class="animate-spin material-symbols">sync</span></div>
      </div>
    </div>
  `;
  document.body.appendChild(modal);

  const closeBtn = modal.querySelector('#close-share-modal');
  closeBtn.onclick = () => modal.remove();

  const body = modal.querySelector('#share-modal-body');

  async function checkAndRender() {
    try {
      const shares = await request('GET', '/api/shares');
      const itemShare = shares.find(s => s.libraryItemId === item.id);

      if (itemShare) {
        const shareUrl = `${window.location.origin}/s/${itemShare.id}`;
        body.innerHTML = `
          <div class="space-y-4">
            <p class="text-xs text-black-100">An active public share link exists for this item.</p>
            
            <div>
              <label class="block text-[0.7rem] uppercase font-semibold text-black-100 mb-1.5 font-bold">Public Share URL</label>
              <div class="flex space-x-2">
                <input type="text" readonly id="share-link-url" value="${shareUrl}" class="flex-grow bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none text-xs cursor-text select-all">
                <button id="copy-share-link-btn" class="bg-accent hover:opacity-90 text-primary font-bold px-3 py-2 rounded flex items-center justify-center space-x-1 text-xs">
                  <span class="material-symbols text-base font-bold">content_copy</span>
                  <span>Copy</span>
                </button>
              </div>
            </div>

            <div class="text-xs text-black-100 space-y-1">
              <div><span class="font-semibold text-white">Downloadable:</span> ${itemShare.isDownloadable ? 'Yes' : 'No'}</div>
              <div><span class="font-semibold text-white">Password Protected:</span> ${itemShare.hasPassword ? 'Yes' : 'No'}</div>
              <div><span class="font-semibold text-white">Expires:</span> ${itemShare.expiresAt ? (window.formatDateTime ? window.formatDateTime(itemShare.expiresAt) : new Date(itemShare.expiresAt).toLocaleString()) : 'Never'}</div>
            </div>

            <div class="pt-2">
              <button id="delete-share-link-btn" class="w-full bg-red-950 hover:bg-red-900 border border-red-500 text-red-100 font-bold py-2 rounded text-xs flex items-center justify-center space-x-1.5 transition-colors">
                <span class="material-symbols text-base">delete</span>
                <span>Remove Share Link</span>
              </button>
            </div>
          </div>
        `;

        const urlInput = body.querySelector('#share-link-url');
        urlInput.onclick = () => urlInput.select();

        body.querySelector('#copy-share-link-btn').onclick = async () => {
          try {
            await navigator.clipboard.writeText(shareUrl);
            alert('Share link copied to clipboard!');
          } catch (err) {
            alert('Failed to copy share link: ' + err.message);
          }
        };

        body.querySelector('#delete-share-link-btn').onclick = async () => {
          try {
            await request('DELETE', `/api/share/mediaitem/${itemShare.id}`);
            alert('Share link removed successfully');
            checkAndRender();
          } catch (err) {
            alert('Failed to delete share link: ' + err.message);
          }
        };

      } else {
        body.innerHTML = `
          <form id="create-share-form" class="space-y-4">
            <div>
              <label class="block text-[0.7rem] uppercase font-semibold text-black-100 mb-1.5">Expires In</label>
              <select id="share-duration" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent text-xs">
                <option value="3600000">1 Hour</option>
                <option value="86400000" selected>1 Day</option>
                <option value="604800000">7 Days</option>
                <option value="2592000000">30 Days</option>
                <option value="0">Never</option>
              </select>
            </div>

            <div class="flex items-center space-x-2">
              <input type="checkbox" id="share-allow-download" checked class="rounded border-black-300 text-accent focus:ring-accent bg-black-500 h-4 w-4">
              <label for="share-allow-download" class="text-xs font-medium text-white">Allow downloads</label>
            </div>

            <div class="space-y-2">
              <div class="flex items-center space-x-2">
                <input type="checkbox" id="share-require-password" class="rounded border-black-300 text-accent focus:ring-accent bg-black-500 h-4 w-4">
                <label for="share-require-password" class="text-xs font-medium text-white">Password protect share link</label>
              </div>
              <div id="share-password-field-container" class="hidden pl-6">
                <input type="password" id="share-password" placeholder="Enter share password" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent text-xs">
              </div>
            </div>

            <button type="submit" class="w-full bg-accent hover:opacity-90 text-primary font-bold py-2.5 rounded transition duration-150 text-xs flex items-center justify-center space-x-1.5 mt-4">
              <span class="material-symbols text-base font-bold">link</span>
              <span>Generate Share Link</span>
            </button>
          </form>
        `;

        const passwordCheckbox = body.querySelector('#share-require-password');
        const passwordContainer = body.querySelector('#share-password-field-container');
        const passwordInput = body.querySelector('#share-password');

        passwordCheckbox.onchange = () => {
          if (passwordCheckbox.checked) {
            passwordContainer.classList.remove('hidden');
            passwordInput.required = true;
          } else {
            passwordContainer.classList.add('hidden');
            passwordInput.required = false;
            passwordInput.value = '';
          }
        };

        const form = body.querySelector('#create-share-form');
        form.onsubmit = async (e) => {
          e.preventDefault();
          
          const durationVal = parseInt(body.querySelector('#share-duration').value, 10);
          const isDownloadable = body.querySelector('#share-allow-download').checked;
          const passwordVal = passwordInput.value;

          const slug = generateSlug();
          let expiresAt = 0;
          if (durationVal > 0) {
            expiresAt = Date.now() + durationVal;
          }

          try {
            await request('POST', '/api/share/mediaitem', {
              slug: slug,
              mediaItemId: item.id,
              mediaItemType: item.mediaType,
              expiresAt: expiresAt,
              isDownloadable: isDownloadable,
              password: passwordVal
            });

            alert('Share link generated successfully!');
            checkAndRender();
          } catch (err) {
            alert('Failed to generate share link: ' + err.message);
          }
        };
      }
    } catch (err) {
      console.error(err);
      body.innerHTML = `<div class="text-red-500 text-xs text-center py-4">Failed to load share settings: ${err.message}</div>`;
    }
  }

  function generateSlug(length = 10) {
    const chars = 'abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789';
    let result = '';
    for (let i = 0; i < length; i++) {
      result += chars.charAt(Math.floor(Math.random() * chars.length));
    }
    return result;
  }

  checkAndRender();
}

