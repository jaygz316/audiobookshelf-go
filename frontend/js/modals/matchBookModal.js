import { request } from '../api.js';
import { escapeHtml, getDiffOldHtml, getDiffNewHtml } from '../itemDetails.js';

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

  const token = localStorage.getItem('token') || '';
  const ts = item.updatedAt || item.addedAt || Date.now();
  const currentCoverUrl = `/api/items/${item.id}/cover?token=${token}&ts=${ts}`;

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
    cover: { label: 'Cover Image', current: currentCoverUrl, activeSource: 'current', options: { current: currentCoverUrl } }
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
    <div class="bg-primary border border-black-300 w-full max-w-4xl p-6 rounded-md shadow-2xl space-y-4 flex flex-col max-h-[90vh]">
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
      <div id="match-results-container" class="flex-grow overflow-y-auto border border-black-300/30 rounded-md p-3 bg-black-500/20 space-y-3 min-h-[150px] max-h-[25vh] no-scroll">
        <p class="text-xs text-black-100 text-center py-6">Enter search criteria and click Search.</p>
      </div>

      <!-- Selected Result Details (Import Options) -->
      <div id="match-details-container" class="flex-grow flex flex-col border-t border-black-400 pt-3 hidden space-y-2 overflow-hidden">
        <h4 class="text-xs uppercase font-bold text-white tracking-wider flex items-center space-x-1.5 flex-shrink-0">
          <span class="material-symbols text-sm text-accent">difference</span>
          <span>Granular Metadata Comparison (Click Card to Select Source)</span>
        </h4>
        <div class="hidden md:grid grid-cols-12 gap-3 px-3 py-1 border-b border-black-400/40 text-[10px] text-black-100 uppercase font-bold tracking-wider flex-shrink-0">
          <div class="col-span-2">Field</div>
          <div class="col-span-5">Current Local Value (Blue)</div>
          <div class="col-span-5">Incoming Provider Value (Gold)</div>
        </div>
        <div id="match-fields-checkboxes" class="flex-grow overflow-y-auto p-3 bg-black-500/30 border border-black-300/30 rounded-md space-y-3 no-scroll">
          <!-- Dynamically populated side-by-side cards -->
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
          updateFieldOptions(selectedResult, providerSelect.value);
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

  const updateCardSelection = (key) => {
    const activeSource = fieldValues[key].activeSource;
    const currentCard = document.getElementById(`card-current-${key}`);
    const incomingCard = document.getElementById(`card-incoming-${key}`);
    if (!currentCard || !incomingCard) return;

    const currentCheck = currentCard.querySelector('.check-icon');
    const incomingCheck = incomingCard.querySelector('.check-icon');

    if (activeSource === 'current') {
      currentCard.className = 'col-span-12 md:col-span-5 p-3 rounded border text-xs cursor-pointer transition-all duration-150 flex flex-col justify-between min-h-[50px] select-none bg-blue-500/10 border-blue-500/50 text-blue-300 ring-1 ring-blue-500/30';
      if (currentCheck) currentCheck.classList.remove('hidden');

      incomingCard.className = 'col-span-12 md:col-span-5 p-3 rounded border text-xs cursor-pointer transition-all duration-150 flex flex-col justify-between min-h-[50px] select-none bg-black-600/30 border-black-400/30 text-black-200 opacity-60 hover:opacity-90 hover:border-black-400';
      if (incomingCheck) incomingCheck.classList.add('hidden');
    } else {
      currentCard.className = 'col-span-12 md:col-span-5 p-3 rounded border text-xs cursor-pointer transition-all duration-150 flex flex-col justify-between min-h-[50px] select-none bg-black-600/30 border-black-400/30 text-black-200 opacity-60 hover:opacity-90 hover:border-black-400';
      if (currentCheck) currentCheck.classList.add('hidden');

      incomingCard.className = 'col-span-12 md:col-span-5 p-3 rounded border text-xs cursor-pointer transition-all duration-150 flex flex-col justify-between min-h-[50px] select-none bg-accent/10 border-accent/50 text-accent ring-1 ring-accent/30';
      if (incomingCheck) incomingCheck.classList.remove('hidden');
    }
  };

  // Populate checkboxes
  function renderMergedFieldsTable() {
    detailsContainer.classList.remove('hidden');
    importBtn.classList.remove('hidden');
    importBtn.textContent = 'Import Selected';

    const providerName = providerSelect.options[providerSelect.selectedIndex]?.text || providerSelect.value;

    checkboxesContainer.innerHTML = Object.entries(fieldValues).map(([key, f]) => {
      const optionKeys = Object.keys(f.options).filter(optKey => {
        const val = f.options[optKey];
        return val !== undefined && val !== null && String(val).trim().length > 0;
      });

      // If only one option (current) and it's empty, we don't show it
      if (optionKeys.length <= 1 && (!f.current || String(f.current).trim().length === 0)) {
        return '';
      }

      const currentVal = f.options['current'] || '';
      const incomingVal = f.options[providerSelect.value] || '';

      let currentDisplay = '';
      let incomingDisplay = '';

      if (key === 'cover') {
        currentDisplay = currentVal 
          ? `<div class="flex items-center justify-center p-1"><img src="${escapeHtml(currentVal)}" class="w-16 h-24 object-cover rounded border border-black-400/50 shadow-md" onerror="this.src='assets/images/logo.png'"></div>`
          : `<div class="text-[10px] text-black-200 italic flex items-center justify-center h-24">No Local Cover</div>`;
        incomingDisplay = incomingVal
          ? `<div class="flex items-center justify-center p-1"><img src="${escapeHtml(incomingVal)}" class="w-16 h-24 object-cover rounded border border-black-400/50 shadow-md" onerror="this.src='assets/images/logo.png'"></div>`
          : `<div class="text-[10px] text-black-200 italic flex items-center justify-center h-24">No Incoming Cover</div>`;
      } else if (key === 'description') {
        currentDisplay = currentVal 
          ? `<div class="max-h-[80px] overflow-y-auto text-[11px] leading-relaxed no-scroll select-text cursor-text">${getDiffOldHtml(currentVal, incomingVal)}</div>`
          : `<span class="text-[11px] text-black-200 italic">Empty</span>`;
        incomingDisplay = incomingVal 
          ? `<div class="max-h-[80px] overflow-y-auto text-[11px] leading-relaxed no-scroll select-text cursor-text">${getDiffNewHtml(currentVal, incomingVal)}</div>`
          : `<span class="text-[11px] text-black-200 italic">Empty</span>`;
      } else {
        currentDisplay = currentVal 
          ? `<div class="text-xs break-words">${getDiffOldHtml(currentVal, incomingVal)}</div>`
          : `<span class="text-[11px] text-black-200 italic">Empty</span>`;
        incomingDisplay = incomingVal 
          ? `<div class="text-xs break-words">${getDiffNewHtml(currentVal, incomingVal)}</div>`
          : `<span class="text-[11px] text-black-200 italic">Empty</span>`;
      }

      return `
        <div class="grid grid-cols-12 gap-3 items-center border-b border-black-400/20 pb-3 last:border-b-0 last:pb-0">
          <div class="col-span-12 md:col-span-2 text-xs font-bold text-white uppercase tracking-wider md:pr-2">
            ${escapeHtml(f.label)}
          </div>
          
          <div id="card-current-${key}" class="col-span-12 md:col-span-5 p-3 rounded border text-xs cursor-pointer transition-all duration-150 flex flex-col justify-between min-h-[50px] relative select-none">
            <div class="text-[9px] text-black-100 uppercase font-semibold mb-1 flex items-center justify-between">
              <span>Local Value</span>
              <span class="check-icon material-symbols text-sm hidden">check_circle</span>
            </div>
            <div class="text-white font-medium flex-grow flex items-center">
              <div class="w-full">${currentDisplay}</div>
            </div>
          </div>

          <div id="card-incoming-${key}" class="col-span-12 md:col-span-5 p-3 rounded border text-xs cursor-pointer transition-all duration-150 flex flex-col justify-between min-h-[50px] relative select-none">
            <div class="text-[9px] text-black-100 uppercase font-semibold mb-1 flex items-center justify-between">
              <span>Incoming (${escapeHtml(providerName)})</span>
              <span class="check-icon material-symbols text-sm hidden font-bold">check_circle</span>
            </div>
            <div class="text-white font-medium flex-grow flex items-center">
              <div class="w-full">${incomingDisplay}</div>
            </div>
          </div>
        </div>
      `;
    }).join('');

    // Attach click listeners to cards and update UI selection states
    Object.keys(fieldValues).forEach(key => {
      const currentCard = document.getElementById(`card-current-${key}`);
      const incomingCard = document.getElementById(`card-incoming-${key}`);

      if (currentCard && incomingCard) {
        currentCard.onclick = () => {
          fieldValues[key].activeSource = 'current';
          updateCardSelection(key);
        };

        incomingCard.onclick = () => {
          fieldValues[key].activeSource = providerSelect.value;
          updateCardSelection(key);
        };

        // Initialize state
        updateCardSelection(key);
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

export function triggerMatchBookModal(item, libraryId, onSaveSuccess) {
  triggerMatchModal(item, libraryId, 'metadata', onSaveSuccess);
}

export function triggerMatchCoverModal(item, libraryId, onSaveSuccess) {
  triggerMatchModal(item, libraryId, 'cover', onSaveSuccess);
}

