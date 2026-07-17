import { request } from './api.js';
import { loadDashboard } from './dashboard.js';

export function initSearchPresets() {
  const saveBtn = document.getElementById('save-preset-btn');
  if (!saveBtn) return;

  saveBtn.onclick = (e) => {
    e.stopPropagation();
    openSavePresetModal();
  };

  renderPresetPills();

  window.addEventListener('library-changed', () => {
    renderPresetPills();
  });
}

function getActiveLibraryId() {
  return localStorage.getItem('activeLibraryId') || '';
}

function getPresetsKey() {
  const libId = getActiveLibraryId();
  return `presets-${libId}`;
}

export function getSavedPresets() {
  try {
    return JSON.parse(localStorage.getItem(getPresetsKey())) || [];
  } catch (e) {
    return [];
  }
}

function savePresets(presets) {
  localStorage.setItem(getPresetsKey(), JSON.stringify(presets));
  renderPresetPills();
}

function openSavePresetModal() {
  // Check if modal already exists in DOM
  let modal = document.getElementById('save-preset-modal');
  if (!modal) {
    modal = document.createElement('div');
    modal.id = 'save-preset-modal';
    modal.className = 'fixed inset-0 bg-black/80 flex items-center justify-center z-[100] transition-opacity duration-200 opacity-0 pointer-events-none';
    modal.innerHTML = `
      <div class="bg-primary border border-black-400/60 rounded-lg max-w-sm w-full mx-4 shadow-2xl overflow-hidden transform scale-95 transition-transform duration-200">
        <div class="px-5 py-4 border-b border-black-400/30 flex items-center justify-between">
          <h3 class="text-sm font-semibold text-white">Save View Preset</h3>
          <button id="close-save-preset-modal" class="text-black-100 hover:text-white transition-colors">
            <span class="material-symbols text-lg font-variation-normal">close</span>
          </button>
        </div>
        <div class="p-5 space-y-4">
          <div>
            <label class="block text-xs text-black-100 mb-1.5 font-medium">Preset Name</label>
            <input type="text" id="preset-name-input" placeholder="e.g. Unstarted Sci-Fi" class="w-full bg-black-600 border border-black-400/55 rounded px-3 py-2 text-sm text-white focus:outline-none focus:border-accent transition duration-150">
          </div>
          <div class="text-xs text-black-200 leading-relaxed bg-black-500/20 p-2.5 rounded border border-black-500/20">
            This will save the current active Filter, Sort, and Sort Direction.
          </div>
        </div>
        <div class="px-5 py-3.5 bg-black-500/10 border-t border-black-400/30 flex justify-end space-x-2">
          <button id="cancel-save-preset" class="px-4 py-2 rounded text-xs text-black-100 hover:text-white hover:bg-black-600 transition duration-150">Cancel</button>
          <button id="submit-save-preset" class="px-4 py-2 bg-accent hover:bg-accent/80 rounded text-xs text-white transition duration-150 font-medium">Save Preset</button>
        </div>
      </div>
    `;
    document.body.appendChild(modal);

    const closeBtn = modal.querySelector('#close-save-preset-modal');
    const cancelBtn = modal.querySelector('#cancel-save-preset');
    const submitBtn = modal.querySelector('#submit-save-preset');
    const nameInput = modal.querySelector('#preset-name-input');

    const closeModal = () => {
      modal.classList.add('opacity-0', 'pointer-events-none');
      modal.querySelector('.transform').classList.remove('scale-100');
      modal.querySelector('.transform').classList.add('scale-95');
    };

    closeBtn.onclick = closeModal;
    cancelBtn.onclick = closeModal;
    modal.onclick = (e) => {
      if (e.target === modal) closeModal();
    };

    submitBtn.onclick = () => {
      const name = nameInput.value.trim();
      if (!name) {
        alert('Please enter a name for the preset.');
        return;
      }

      const filterBy = localStorage.getItem('library-filterBy') || '';
      const sortBy = localStorage.getItem('library-sortBy') || 'media.metadata.title';
      const sortDesc = localStorage.getItem('library-sortDesc') === 'true';

      const newPreset = {
        id: 'preset-' + Date.now(),
        name,
        filterBy,
        sortBy,
        sortDesc
      };

      const presets = getSavedPresets();
      presets.push(newPreset);
      savePresets(presets);

      nameInput.value = '';
      closeModal();
    };
  }

  // Show modal
  modal.classList.remove('opacity-0', 'pointer-events-none');
  modal.querySelector('.transform').classList.remove('scale-95');
  modal.querySelector('.transform').classList.add('scale-100');
  modal.querySelector('#preset-name-input').focus();
}

export function renderPresetPills() {
  const container = document.getElementById('presets-pills-container');
  if (!container) return;

  const presets = getSavedPresets();
  if (presets.length === 0) {
    container.innerHTML = '';
    return;
  }

  const currentFilterBy = localStorage.getItem('library-filterBy') || '';
  const currentSortBy = localStorage.getItem('library-sortBy') || 'media.metadata.title';
  const currentSortDesc = localStorage.getItem('library-sortDesc') === 'true';

  container.innerHTML = presets.map(preset => {
    const isActive = preset.filterBy === currentFilterBy &&
                     preset.sortBy === currentSortBy &&
                     preset.sortDesc === currentSortDesc;

    const activeClasses = isActive 
      ? 'bg-accent border-accent text-primary font-semibold shadow-sm'
      : 'border border-black-300/30 text-black-100 hover:text-white hover:bg-black-600/40';

    return `
      <div class="preset-pill flex items-center px-3 py-1 rounded-full text-[11px] transition-all duration-150 cursor-pointer ${activeClasses}" data-id="${preset.id}">
        <span>${preset.name}</span>
        <button class="delete-preset-btn ml-1.5 ${isActive ? 'text-primary/70 hover:text-primary' : 'hover:text-red-400'} focus:outline-none transition-colors" data-id="${preset.id}">
          <span class="material-symbols text-[12px] pt-[2px] font-variation-normal">close</span>
        </button>
      </div>
    `;
  }).join('');

  // Wire click events
  container.querySelectorAll('.preset-pill').forEach(pill => {
    const presetId = pill.getAttribute('data-id');
    const preset = presets.find(p => p.id === presetId);
    if (!preset) return;

    pill.onclick = (e) => {
      // If delete button was clicked, don't trigger apply
      if (e.target.closest('.delete-preset-btn')) return;

      localStorage.setItem('library-filterBy', preset.filterBy);
      localStorage.setItem('library-sortBy', preset.sortBy);
      localStorage.setItem('library-sortDesc', preset.sortDesc.toString());

      // Update Filter Label in dropdown and highlight button if active
      if (window.updateFilterLabelGlobal) {
        window.updateFilterLabelGlobal(preset.filterBy);
      } else {
        const labelEl = document.getElementById('filter-selected-label');
        if (labelEl) {
          labelEl.textContent = getFriendlyFilterLabel(preset.filterBy);
        }
      }

      // Update Sort Label and check icons
      const sortLabelEl = document.getElementById('sort-selected-label');
      const sortLabels = {
        "media.metadata.title": "Sort: Title",
        "media.metadata.authorName": "Sort: Author",
        "media.metadata.publishedYear": "Sort: Year",
        "addedAt": "Sort: Date Added",
        "media.duration": "Sort: Duration",
        "random": "Sort: Random"
      };
      if (sortLabelEl) {
        sortLabelEl.textContent = sortLabels[preset.sortBy] || 'Sort: Title';
      }

      // Update Sort checkmark and order icon in DOM
      const sortMenu = document.getElementById('sort-dropdown-menu');
      if (sortMenu) {
        sortMenu.querySelectorAll('.sort-option-btn').forEach(btn => {
          const check = btn.querySelector('.check-icon');
          if (btn.getAttribute('data-value') === preset.sortBy) {
            check?.classList.remove('hidden');
            btn.classList.add('text-accent', 'font-medium');
          } else {
            check?.classList.add('hidden');
            btn.classList.remove('text-accent', 'font-medium');
          }
        });
      }

      const sortOrderIcon = document.getElementById('sort-order-icon');
      if (sortOrderIcon) {
        sortOrderIcon.textContent = preset.sortDesc ? 'arrow_downward' : 'arrow_upward';
      }

      // Refresh pills rendering so active state shows correctly
      renderPresetPills();

      // Trigger reload
      const libId = getActiveLibraryId();
      if (libId) loadDashboard(libId);
    };
  });

  // Wire delete events
  container.querySelectorAll('.delete-preset-btn').forEach(btn => {
    btn.onclick = (e) => {
      e.stopPropagation();
      const presetId = btn.getAttribute('data-id');
      const filtered = presets.filter(p => p.id !== presetId);
      savePresets(filtered);
    };
  });
}

// Fallback or helper for friendly label inside presets
function getFriendlyFilterLabel(val) {
  if (!val) return 'Filter: All';
  const parts = val.split('.');
  const category = parts[0];
  const subVal = parts[1];

  const getDecodedSubVal = (s) => {
    try {
      return decodeURIComponent(escape(atob(s)));
    } catch (e) {
      try {
        return decodeURIComponent(s);
      } catch (err) {
        return s;
      }
    }
  };

  switch (category) {
    case 'progress':
      if (subVal === 'not-started') return 'Unstarted';
      if (subVal === 'in-progress') return 'In Progress';
      if (subVal === 'finished') return 'Completed';
      break;
    case 'authors':
      return 'Author';
    case 'series':
      if (subVal === 'no-series') return 'No Series';
      return 'Series';
    case 'narrators':
      return `Narrator: ${getDecodedSubVal(subVal)}`;
    case 'genres':
      return `Genre: ${getDecodedSubVal(subVal)}`;
    case 'tags':
      return `Tag: ${getDecodedSubVal(subVal)}`;
    case 'publishers':
      return `Publisher: ${getDecodedSubVal(subVal)}`;
    case 'languages':
      return `Language: ${getDecodedSubVal(subVal)}`;
    case 'decades':
      return `Decade: ${getDecodedSubVal(subVal)}s`;
    case 'duration':
      if (subVal === 'under-1h') return 'Duration: < 1h';
      if (subVal === '1h-5h') return 'Duration: 1-5h';
      if (subVal === '5h-10h') return 'Duration: 5-10h';
      if (subVal === 'over-10h') return 'Duration: > 10h';
      break;
    case 'missing':
      return 'Missing / Invalid';
  }
  return 'Filtered';
}
