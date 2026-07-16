import { request } from '../api.js';
import { escapeHtml, splitCommaList } from '../itemDetails.js';

export function triggerEditItemDetailsModal(item, libraryId, onSaveSuccess) {
  const mediaType = item.mediaType || 'book';
  let metadata = {};
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
    metadata = item.media.metadata || {};
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

  const lockedFields = metadata.lockedFields || [];
  const currentLockedFields = new Set(lockedFields);

  const getFieldLabel = (field) => {
    switch (field) {
      case 'title': return 'Title';
      case 'subtitle': return 'Subtitle';
      case 'authors': return mediaType === 'podcast' ? 'Author / Host' : 'Author(s) (comma separated)';
      case 'narrators': return 'Narrator(s) (comma separated)';
      case 'series': return 'Series Name & Sequence';
      case 'description': return 'Description';
      case 'publisher': return 'Publisher';
      case 'publishedYear': return 'Publish Year';
      case 'publishedDate': return 'Publish Date';
      case 'isbn': return 'ISBN';
      case 'asin': return 'ASIN';
      case 'language': return 'Language';
      case 'genres': return 'Genres (comma separated)';
      case 'tags': return 'Tags (comma separated)';
      default: return field;
    }
  };

  const getLockIconHtml = (field) => {
    const isLocked = currentLockedFields.has(field);
    const icon = isLocked ? 'lock' : 'lock_open';
    const colorClass = isLocked ? 'text-yellow-400 hover:text-yellow-300' : 'text-black-200 hover:text-accent';
    return `
      <div class="flex items-center justify-between mb-1.5">
        <label class="block text-[0.7rem] uppercase font-semibold text-black-100 mb-0">${getFieldLabel(field)}</label>
        <button type="button" class="metadata-lock-btn focus:outline-none transition-colors ${colorClass}" data-field="${field}">
          <span class="material-symbols text-sm select-none pointer-events-none">${icon}</span>
        </button>
      </div>
    `;
  };

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
            ${getLockIconHtml('title')}
            <input type="text" id="edit-item-title" required value="${escapeHtml(title)}" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent text-xs">
          </div>
          <div>
            ${getLockIconHtml('subtitle')}
            <input type="text" id="edit-item-subtitle" value="${escapeHtml(subtitle)}" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent text-xs">
          </div>
        </div>

        <!-- Authors & Narrators -->
        <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
          <div>
            ${getLockIconHtml('authors')}
            <input type="text" id="edit-item-authors" value="${escapeHtml(authors.join(', '))}" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent text-xs">
          </div>
          ${mediaType === 'book' ? `
            <div>
              ${getLockIconHtml('narrators')}
              <input type="text" id="edit-item-narrators" value="${escapeHtml(narrators.join(', '))}" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent text-xs">
            </div>
          ` : ''}
        </div>

        <!-- Series (Only Book) -->
        ${mediaType === 'book' ? `
          <div class="grid grid-cols-3 gap-4">
            <div class="col-span-2">
              ${getLockIconHtml('series')}
              <input type="text" id="edit-item-series" value="${escapeHtml(seriesName)}" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent text-xs">
            </div>
            <div>
              <div class="flex items-center justify-between mb-1.5">
                <label class="block text-[0.7rem] uppercase font-semibold text-black-100 mb-0">Sequence</label>
              </div>
              <input type="text" id="edit-item-sequence" value="${escapeHtml(seriesSequence)}" placeholder="e.g. 1, 1.5" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent text-xs">
            </div>
          </div>
        ` : ''}

        <!-- Description -->
        <div>
          ${getLockIconHtml('description')}
          <textarea id="edit-item-description" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent text-xs h-24 resize-none">${escapeHtml(description)}</textarea>
        </div>

        <!-- Publisher & Dates -->
        <div class="grid grid-cols-3 gap-4">
          <div>
            ${getLockIconHtml('publisher')}
            <input type="text" id="edit-item-publisher" value="${escapeHtml(publisher)}" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent text-xs">
          </div>
          <div>
            ${getLockIconHtml('publishedYear')}
            <input type="text" id="edit-item-pubyear" value="${escapeHtml(publishedYear)}" placeholder="e.g. 2023" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent text-xs">
          </div>
          <div>
            ${getLockIconHtml('publishedDate')}
            <input type="text" id="edit-item-pubdate" value="${escapeHtml(publishedDate)}" placeholder="YYYY-MM-DD" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent text-xs">
          </div>
        </div>

        <!-- ISBN, ASIN, Language -->
        <div class="grid grid-cols-3 gap-4">
          <div>
            ${getLockIconHtml('isbn')}
            <input type="text" id="edit-item-isbn" value="${escapeHtml(isbn)}" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent text-xs">
          </div>
          <div>
            ${getLockIconHtml('asin')}
            <input type="text" id="edit-item-asin" value="${escapeHtml(asin)}" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent text-xs">
          </div>
          <div>
            ${getLockIconHtml('language')}
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
            ${getLockIconHtml('genres')}
            <input type="text" id="edit-item-genres" value="${escapeHtml(genres.join(', '))}" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent text-xs">
          </div>
          <div>
            ${getLockIconHtml('tags')}
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

  // Bind lock click handlers
  modal.querySelectorAll('.metadata-lock-btn').forEach(btn => {
    btn.onclick = (e) => {
      e.preventDefault();
      const field = btn.getAttribute('data-field');
      const iconSpan = btn.querySelector('.material-symbols');
      if (currentLockedFields.has(field)) {
        currentLockedFields.delete(field);
        iconSpan.textContent = 'lock_open';
        btn.className = 'metadata-lock-btn focus:outline-none transition-colors text-black-200 hover:text-accent';
      } else {
        currentLockedFields.add(field);
        iconSpan.textContent = 'lock';
        btn.className = 'metadata-lock-btn focus:outline-none transition-colors text-yellow-400 hover:text-yellow-300';
      }
    };
  });

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
      tags: splitCommaList(document.getElementById('edit-item-tags').value),
      lockedFields: Array.from(currentLockedFields)
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
