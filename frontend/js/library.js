// js/library.js

let libraries = [];
let activeLibraryId = null;

export function initLibrary(payload) {
  libraries = payload.libraries || [];
  
  if (libraries.length === 0) {
    console.warn('No accessible libraries found');
    const selectedNameEl = document.getElementById('library-dropdown-selected-name');
    if (selectedNameEl) {
      selectedNameEl.textContent = 'None';
    }
    return;
  }

  // Resolve active library ID
  let storedId = localStorage.getItem('activeLibraryId');
  const libraryExists = libraries.some(lib => lib.id === storedId);
  if (!libraryExists) {
    storedId = payload.userDefaultLibraryId || libraries[0].id;
  }
  
  setActiveLibrary(storedId);
  setupDropdown();
}

export function getActiveLibraryId() {
  if (!activeLibraryId && libraries.length > 0) {
    activeLibraryId = localStorage.getItem('activeLibraryId') || libraries[0].id;
  }
  return activeLibraryId;
}

export function getLibrariesList() {
  return libraries;
}

function setActiveLibrary(libraryId) {
  activeLibraryId = libraryId;
  localStorage.setItem('activeLibraryId', libraryId);
  
  const selectedLib = libraries.find(lib => lib.id === libraryId);
  const selectedNameEl = document.getElementById('library-dropdown-selected-name');
  if (selectedNameEl && selectedLib) {
    selectedNameEl.textContent = selectedLib.name;
  }

  // Dispatch global event for dashboard to update
  window.dispatchEvent(new CustomEvent('library-changed', { detail: { libraryId } }));
}

function setupDropdown() {
  const dropdownBtn = document.getElementById('library-dropdown-btn');
  const dropdownMenu = document.getElementById('library-dropdown-menu');

  if (!dropdownBtn || !dropdownMenu) return;

  // Render menu items
  dropdownMenu.innerHTML = '';
  libraries.forEach(lib => {
    const item = document.createElement('button');
    item.className = 'w-full text-left flex items-center space-x-2 px-4 py-2 text-sm text-black-50 hover:bg-black-400 hover:text-white transition-colors focus:outline-none';
    item.type = 'button';
    
    // Choose icon based on mediaType or icon property
    let iconName = 'local_library';
    if (lib.icon === 'audiobooks' || lib.mediaType === 'book') {
      iconName = 'local_library';
    } else if (lib.icon === 'podcasts' || lib.mediaType === 'podcast') {
      iconName = 'podcasts';
    }
    
    item.innerHTML = `
      <span class="material-symbols text-lg">${iconName}</span>
      <span>${escapeHtml(lib.name)}</span>
    `;

    item.addEventListener('click', (e) => {
      e.stopPropagation();
      setActiveLibrary(lib.id);
      dropdownMenu.classList.add('hidden');
    });

    dropdownMenu.appendChild(item);
  });

  // Click button toggles dropdown
  dropdownBtn.onclick = (e) => {
    e.stopPropagation();
    dropdownMenu.classList.toggle('hidden');
  };

  // Close dropdown when clicking elsewhere
  document.addEventListener('click', () => {
    dropdownMenu.classList.add('hidden');
  });
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
