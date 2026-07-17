// frontend/js/opml.js

import { request } from './api.js';
import { showToast } from './toast.js';

export function openOPMLModal(libraryId) {
  const modal = document.createElement('div');
  modal.className = 'fixed inset-0 bg-black-900/80 z-50 flex items-center justify-center p-4 backdrop-blur-sm';
  
  let activeTab = 'export'; // Default active tab
  let parsedFeeds = [];     // Stores parsed podcast feeds
  let isImporting = false;  // Tracks loading state for import process

  const renderModalContent = () => {
    modal.innerHTML = `
      <div class="bg-primary border border-black-300 w-full max-w-lg p-6 rounded-md shadow-lg flex flex-col max-h-[90vh]">
        <div class="flex justify-between items-center border-b border-black-400 pb-3 mb-4 flex-shrink-0">
          <h3 class="text-lg font-bold text-white flex items-center gap-2">
            <span class="material-symbols text-accent">import_export</span>
            OPML Podcast Subscriptions
          </h3>
          <button id="close-opml-modal-btn" class="text-black-100 hover:text-white transition-colors">
            <span class="material-symbols">close</span>
          </button>
        </div>

        <!-- Tabs -->
        <div class="flex border-b border-black-400 mb-4 flex-shrink-0">
          <button id="opml-tab-export" class="px-4 py-2 border-b-2 font-semibold text-sm focus:outline-none transition-colors ${
            activeTab === 'export' ? 'border-accent text-accent' : 'border-transparent text-black-50 hover:text-white'
          }">Export</button>
          <button id="opml-tab-import" class="px-4 py-2 border-b-2 font-semibold text-sm focus:outline-none transition-colors ${
            activeTab === 'import' ? 'border-accent text-accent' : 'border-transparent text-black-50 hover:text-white'
          }">Import</button>
        </div>

        <!-- Dynamic Content Area -->
        <div class="flex-grow overflow-y-auto space-y-4 pr-1 scrollbar-thin">
          ${activeTab === 'export' ? renderExportTab() : renderImportTab()}
        </div>
      </div>
    `;

    // Attach base events
    modal.querySelector('#close-opml-modal-btn').onclick = () => modal.remove();
    
    const tabExport = modal.querySelector('#opml-tab-export');
    const tabImport = modal.querySelector('#opml-tab-import');
    
    if (tabExport && tabImport) {
      tabExport.onclick = () => {
        if (isImporting) return;
        activeTab = 'export';
        renderModalContent();
      };
      tabImport.onclick = () => {
        if (isImporting) return;
        activeTab = 'import';
        renderModalContent();
      };
    }

    // Attach active tab specific events
    if (activeTab === 'export') {
      const exportBtn = modal.querySelector('#opml-export-action-btn');
      if (exportBtn) {
        exportBtn.onclick = async () => {
          try {
            exportBtn.disabled = true;
            exportBtn.textContent = 'Generating...';
            const opmlText = await request('GET', `/api/libraries/${libraryId}/opml`);
            
            const blob = new Blob([opmlText], { type: 'application/xml' });
            const url = URL.createObjectURL(blob);
            const a = document.createElement('a');
            a.href = url;
            a.download = 'audiobookshelf-podcasts.opml';
            document.body.appendChild(a);
            a.click();
            document.body.removeChild(a);
            URL.revokeObjectURL(url);
            
            showToast('OPML file downloaded successfully', 'success');
          } catch (err) {
            showToast('Failed to export OPML: ' + err.message, 'error');
          } finally {
            exportBtn.disabled = false;
            exportBtn.textContent = 'Export Subscriptions (.opml)';
          }
        };
      }
    } else if (activeTab === 'import') {
      if (isImporting) {
        // Loading state, no events to attach
        return;
      }

      const fileInput = modal.querySelector('#opml-file-input');
      const dropZone = modal.querySelector('#opml-dropzone');
      
      if (fileInput && dropZone) {
        fileInput.onchange = (e) => handleFileSelected(e.target.files[0]);
        
        dropZone.ondragover = (e) => {
          e.preventDefault();
          dropZone.classList.add('border-accent', 'bg-accent/5');
        };
        dropZone.ondragleave = (e) => {
          e.preventDefault();
          dropZone.classList.remove('border-accent', 'bg-accent/5');
        };
        dropZone.ondrop = (e) => {
          e.preventDefault();
          dropZone.classList.remove('border-accent', 'bg-accent/5');
          if (e.dataTransfer.files.length > 0) {
            handleFileSelected(e.dataTransfer.files[0]);
          }
        };
      }

      // If feeds are parsed, wire up list events
      if (parsedFeeds.length > 0) {
        const selectAllCheckbox = modal.querySelector('#opml-select-all');
        const listCheckboxes = modal.querySelectorAll('.opml-feed-checkbox');
        const startImportBtn = modal.querySelector('#opml-start-import-btn');
        
        if (selectAllCheckbox) {
          selectAllCheckbox.onchange = (e) => {
            const checked = e.target.checked;
            listCheckboxes.forEach(cb => {
              cb.checked = checked;
            });
            updateImportButtonState();
          };
        }

        listCheckboxes.forEach(cb => {
          cb.onchange = () => {
            if (selectAllCheckbox) {
              const allChecked = Array.from(listCheckboxes).every(c => c.checked);
              selectAllCheckbox.checked = allChecked;
            }
            updateImportButtonState();
          };
        });

        if (startImportBtn) {
          startImportBtn.onclick = async () => {
            const selectedFeedUrls = Array.from(listCheckboxes)
              .filter(cb => cb.checked)
              .map(cb => cb.value);

            if (selectedFeedUrls.length === 0) {
              showToast('Please select at least one podcast to import', 'error');
              return;
            }

            const autoDownload = modal.querySelector('#opml-auto-download')?.checked || false;

            try {
              isImporting = true;
              renderModalContent();

              await request('POST', '/api/podcasts/opml/create', {
                feeds: selectedFeedUrls,
                libraryId: libraryId,
                autoDownloadEpisodes: autoDownload
              });

              showToast(`Successfully started importing ${selectedFeedUrls.length} podcasts`, 'success');
              modal.remove();
              // Trigger dashboard reload
              window.dispatchEvent(new CustomEvent('library-changed', { detail: { libraryId } }));
            } catch (err) {
              showToast('Failed to import podcasts: ' + err.message, 'error');
              isImporting = false;
              renderModalContent();
            }
          };
        }
      }
    }
  };

  const handleFileSelected = (file) => {
    if (!file) return;
    const reader = new FileReader();
    reader.onload = async (e) => {
      const opmlText = e.target.result;
      try {
        const parseResult = await request('POST', '/api/podcasts/opml/parse', { opmlText });
        parsedFeeds = parseResult.feeds || [];
        if (parsedFeeds.length === 0) {
          showToast('No podcast feeds found in the OPML file', 'warning');
        } else {
          showToast(`Found ${parsedFeeds.length} podcast feeds`, 'success');
        }
        renderModalContent();
      } catch (err) {
        showToast('Failed to parse OPML file: ' + err.message, 'error');
      }
    };
    reader.readAsText(file);
  };

  const updateImportButtonState = () => {
    const listCheckboxes = modal.querySelectorAll('.opml-feed-checkbox');
    const startImportBtn = modal.querySelector('#opml-start-import-btn');
    if (startImportBtn) {
      const checkedCount = Array.from(listCheckboxes).filter(c => c.checked).length;
      startImportBtn.textContent = `Import Selected (${checkedCount})`;
      startImportBtn.disabled = checkedCount === 0;
      if (checkedCount === 0) {
        startImportBtn.classList.add('opacity-50', 'cursor-not-allowed');
      } else {
        startImportBtn.classList.remove('opacity-50', 'cursor-not-allowed');
      }
    }
  };

  const renderExportTab = () => {
    return `
      <div class="space-y-4 py-4 text-center">
        <span class="material-symbols text-6xl text-black-100">download</span>
        <p class="text-sm text-black-50 max-w-sm mx-auto">
          Export all podcast subscriptions in this library to a standard OPML file.
        </p>
        <div class="pt-4">
          <button id="opml-export-action-btn" class="bg-accent hover:opacity-90 text-primary font-bold px-6 py-2.5 rounded-md text-sm transition-all shadow hover:scale-[1.02] duration-200">
            Export Subscriptions (.opml)
          </button>
        </div>
      </div>
    `;
  };

  const renderImportTab = () => {
    if (isImporting) {
      return `
        <div class="flex flex-col items-center justify-center py-12 space-y-4">
          <div class="animate-spin rounded-full h-12 w-12 border-b-2 border-accent"></div>
          <p class="text-sm font-semibold text-white">Importing podcast subscriptions...</p>
          <p class="text-xs text-black-100">This may take a few moments depending on the number of feeds.</p>
        </div>
      `;
    }

    if (parsedFeeds.length > 0) {
      return `
        <div class="space-y-4">
          <div class="flex justify-between items-center">
            <h4 class="text-sm font-semibold text-white">Select Podcasts to Import</h4>
            <label class="flex items-center space-x-1.5 text-xs text-black-50 cursor-pointer">
              <input type="checkbox" id="opml-select-all" checked class="rounded text-accent bg-black-600 border-black-300 focus:ring-accent">
              <span>Select All</span>
            </label>
          </div>

          <div class="border border-black-300 rounded p-2 bg-black-500 max-h-60 overflow-y-auto space-y-1.5 scrollbar-thin">
            ${parsedFeeds.map((feed, idx) => `
              <label class="flex items-start space-x-3 text-xs cursor-pointer hover:bg-black-400 p-2 rounded transition-colors">
                <input type="checkbox" value="${escapeHtml(feed.xmlUrl || feed.feedUrl)}" checked class="opml-feed-checkbox mt-0.5 rounded text-accent bg-black-600 border-black-300 focus:ring-accent">
                <div class="min-w-0 flex-grow">
                  <div class="font-semibold text-white truncate">${escapeHtml(feed.title || 'Unnamed Podcast')}</div>
                  <div class="text-[10px] text-black-100 truncate">${escapeHtml(feed.xmlUrl || feed.feedUrl)}</div>
                </div>
              </label>
            `).join('')}
          </div>

          <div class="pt-2 border-t border-black-400">
            <label class="flex items-center space-x-2 text-xs text-black-50 cursor-pointer">
              <input type="checkbox" id="opml-auto-download" checked class="rounded text-accent bg-black-600 border-black-300 focus:ring-accent">
              <span>Auto-download new episodes</span>
            </label>
          </div>

          <div class="flex justify-end space-x-3 pt-2">
            <button id="opml-cancel-import-btn" class="bg-black-400 hover:bg-black-300 text-white px-4 py-2 rounded text-xs">
              Clear
            </button>
            <button id="opml-start-import-btn" class="bg-accent hover:opacity-90 text-primary font-bold px-4 py-2 rounded text-xs transition-opacity shadow">
              Import Selected (${parsedFeeds.length})
            </button>
          </div>
        </div>
      `;
    }

    return `
      <div id="opml-dropzone" class="border-2 border-dashed border-black-300 hover:border-accent rounded-lg p-8 flex flex-col items-center justify-center cursor-pointer transition-all bg-black-500/10">
        <span class="material-symbols text-4xl text-black-100 mb-2">upload_file</span>
        <p class="text-sm font-semibold text-white">Click or drag OPML file here to upload</p>
        <p class="text-xs text-black-100 mt-1">Supports .opml and .xml files</p>
        <input type="file" id="opml-file-input" accept=".opml,.xml" class="hidden">
      </div>
    `;
  };

  // Drag and drop zone helper handlers
  modal.addEventListener('click', (e) => {
    const dropZone = modal.querySelector('#opml-dropzone');
    if (dropZone && dropZone.contains(e.target)) {
      modal.querySelector('#opml-file-input').click();
    }
    
    const cancelImportBtn = modal.querySelector('#opml-cancel-import-btn');
    if (cancelImportBtn && cancelImportBtn.contains(e.target)) {
      parsedFeeds = [];
      renderModalContent();
    }
  });

  renderModalContent();
  document.body.appendChild(modal);
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
