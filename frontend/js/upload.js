// frontend/js/upload.js

import { request, resolvePath } from './api.js';
import { showToast } from './app.js';

let uploadModal = null;
let fileQueue = []; // Array of { file, path }

export async function openUploadModal(libraryId, initialFiles = []) {
  // If modal is already open, just append new files
  if (uploadModal) {
    if (initialFiles.length > 0) {
      appendFilesToQueue(initialFiles);
    }
    return;
  }

  fileQueue = [];
  if (initialFiles.length > 0) {
    appendFilesToQueue(initialFiles);
  }

  // Fetch library details to get folders
  let libraryFolders = [];
  let libraryName = 'Library';
  try {
    const lib = await request('GET', `/api/libraries/${libraryId}`);
    libraryFolders = lib.folders || [];
    libraryName = lib.name || 'Library';
  } catch (err) {
    console.error('Failed to fetch library details:', err);
    showToast('Failed to fetch library folders: ' + err.message, 'error');
  }

  uploadModal = document.createElement('div');
  uploadModal.className = 'fixed inset-0 bg-black-900/80 z-50 flex items-center justify-center p-4 backdrop-blur-sm';
  
  renderModal(libraryId, libraryName, libraryFolders);
  document.body.appendChild(uploadModal);
}

function appendFilesToQueue(files) {
  // files is array of { file, path }
  files.forEach(item => {
    // Prevent duplicates by checking file name and path
    const exists = fileQueue.some(q => q.path === item.path && q.file.size === item.file.size);
    if (!exists) {
      fileQueue.push(item);
    }
  });
  updateQueueUI();
}

function formatBytes(bytes) {
  if (bytes === 0) return '0 Bytes';
  const k = 1024;
  const sizes = ['Bytes', 'KB', 'MB', 'GB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
}

function updateQueueUI() {
  if (!uploadModal) return;

  const queueContainer = uploadModal.querySelector('#upload-queue-container');
  const queueSummary = uploadModal.querySelector('#upload-queue-summary');
  const uploadBtn = uploadModal.querySelector('#upload-start-btn');

  if (!queueContainer || !queueSummary) return;

  if (fileQueue.length === 0) {
    queueContainer.innerHTML = `
      <div class="flex flex-col items-center justify-center h-32 border border-dashed border-black-300 rounded-md text-black-100 bg-black-500/10">
        <span class="material-symbols text-3xl mb-1">list_alt</span>
        <p class="text-xs">No files added to the queue yet</p>
      </div>
    `;
    queueSummary.textContent = '0 files (0 Bytes)';
    if (uploadBtn) {
      uploadBtn.disabled = true;
      uploadBtn.classList.add('opacity-50', 'cursor-not-allowed');
    }
    return;
  }

  if (uploadBtn) {
    uploadBtn.disabled = false;
    uploadBtn.classList.remove('opacity-50', 'cursor-not-allowed');
  }

  let totalSize = 0;
  queueContainer.innerHTML = '';
  
  // Render list of files in queue
  const list = document.createElement('div');
  list.className = 'space-y-1 max-h-[16rem] overflow-y-auto pr-1 scrollbar-thin';
  
  fileQueue.forEach((item, index) => {
    totalSize += item.file.size;
    
    const row = document.createElement('div');
    row.className = 'flex justify-between items-center bg-black-500/30 hover:bg-black-500/50 border border-black-400/30 px-3 py-2 rounded text-xs text-white transition-colors';
    
    // Split folder structure if nested
    const pathParts = item.path.split('/');
    const filename = pathParts.pop();
    const folderPath = pathParts.join('/');
    
    row.innerHTML = `
      <div class="flex flex-col min-w-0 pr-4">
        <span class="font-medium truncate">${filename}</span>
        ${folderPath ? `<span class="text-[10px] text-black-100 truncate flex items-center gap-0.5"><span class="material-symbols text-[12px]">folder</span> ${folderPath}</span>` : ''}
      </div>
      <div class="flex items-center space-x-3 flex-shrink-0">
        <span class="text-black-100 font-mono">${formatBytes(item.file.size)}</span>
        <button class="remove-queue-item-btn text-red-400 hover:text-red-300 transition-colors focus:outline-none" data-index="${index}">
          <span class="material-symbols text-lg">delete</span>
        </button>
      </div>
    `;
    
    row.querySelector('.remove-queue-item-btn').onclick = (e) => {
      e.stopPropagation();
      fileQueue.splice(index, 1);
      updateQueueUI();
    };

    list.appendChild(row);
  });

  queueContainer.appendChild(list);
  queueSummary.textContent = `${fileQueue.length} file(s) (${formatBytes(totalSize)})`;
}

function renderModal(libraryId, libraryName, folders) {
  if (!uploadModal) return;

  uploadModal.innerHTML = `
    <div class="bg-primary border border-black-300 w-full max-w-2xl p-6 rounded-md shadow-2xl flex flex-col max-h-[90vh]">
      <!-- Header -->
      <div class="flex justify-between items-center border-b border-black-400 pb-3 mb-4 flex-shrink-0">
        <h3 class="text-lg font-bold text-white flex items-center gap-2">
          <span class="material-symbols text-accent">upload</span>
          Upload Content to ${escapeHtml(libraryName)}
        </h3>
        <button id="close-upload-modal-btn" class="text-black-100 hover:text-white transition-colors">
          <span class="material-symbols">close</span>
        </button>
      </div>

      <!-- Main Scrollable Body -->
      <div id="upload-modal-body" class="flex-grow overflow-y-auto pr-1 space-y-4 scrollbar-thin">
        
        <!-- Target Folder Selection -->
        <div class="flex flex-col space-y-1">
          <label for="upload-target-folder" class="text-xs font-semibold text-black-50">Target Library Folder</label>
          <select id="upload-target-folder" class="bg-black-500 text-white border border-black-300 rounded px-3 py-2 text-sm focus:outline-none focus:border-accent">
            ${folders.map(f => `<option value="${f.id}">${escapeHtml(f.path)}</option>`).join('')}
          </select>
        </div>

        <!-- Drag & Drop Zone -->
        <div id="upload-dropzone" class="border-2 border-dashed border-black-300 rounded-lg p-8 flex flex-col items-center justify-center bg-black-500/10 hover:bg-black-500/20 hover:border-accent/50 cursor-pointer transition-all">
          <span class="material-symbols text-4xl text-accent mb-2">cloud_upload</span>
          <p class="text-sm font-medium text-white text-center mb-1">Drag and drop media files or folders here</p>
          <p class="text-xs text-black-100 text-center mb-4">Supports bulk files and nested audio/ebook folder structures</p>
          
          <div class="flex items-center space-x-2">
            <button id="select-files-btn" class="bg-accent hover:bg-accent-hover text-primary font-semibold px-3 py-1.5 rounded text-xs transition-colors focus:outline-none">
              Choose Files
            </button>
            <button id="select-folder-btn" class="bg-black-400 hover:bg-black-300 border border-black-300 text-white font-semibold px-3 py-1.5 rounded text-xs transition-colors focus:outline-none">
              Choose Folder
            </button>
          </div>
          
          <!-- Hidden standard file inputs -->
          <input type="file" id="upload-files-input" multiple class="hidden">
          <input type="file" id="upload-folder-input" webkitdirectory directory multiple class="hidden">
        </div>

        <!-- Selected Queue Header -->
        <div class="flex justify-between items-center border-b border-black-400/50 pb-1.5">
          <h4 class="text-xs font-semibold text-black-50 uppercase tracking-wider">Upload Queue</h4>
          <div class="flex items-center space-x-2">
            <span id="upload-queue-summary" class="text-xs text-black-100 font-mono">0 files (0 Bytes)</span>
            <button id="clear-queue-btn" class="text-xs text-red-400 hover:text-red-300 transition-colors focus:outline-none hidden">Clear All</button>
          </div>
        </div>

        <!-- Selected Queue List Container -->
        <div id="upload-queue-container">
          <!-- Populated dynamically -->
        </div>

      </div>

      <!-- Action Footer -->
      <div id="upload-modal-footer" class="flex justify-end items-center space-x-3 border-t border-black-400 pt-4 mt-4 flex-shrink-0">
        <button id="cancel-upload-btn" class="bg-black-400 hover:bg-black-300 border border-black-300 text-white px-4 py-2 rounded text-sm font-semibold transition-colors focus:outline-none">
          Cancel
        </button>
        <button id="upload-start-btn" class="bg-accent hover:bg-accent-hover text-primary px-5 py-2 rounded text-sm font-semibold transition-colors focus:outline-none opacity-50 cursor-not-allowed" disabled>
          Upload & Scan
        </button>
      </div>
    </div>
  `;

  // Attach Event Handlers
  const closeBtn = uploadModal.querySelector('#close-upload-modal-btn');
  const cancelBtn = uploadModal.querySelector('#cancel-upload-btn');
  const clearBtn = uploadModal.querySelector('#clear-queue-btn');
  const uploadStartBtn = uploadModal.querySelector('#upload-start-btn');
  const selectFilesBtn = uploadModal.querySelector('#select-files-btn');
  const selectFolderBtn = uploadModal.querySelector('#select-folder-btn');
  const filesInput = uploadModal.querySelector('#upload-files-input');
  const folderInput = uploadModal.querySelector('#upload-folder-input');
  const dropZone = uploadModal.querySelector('#upload-dropzone');

  const closeModal = () => {
    uploadModal.remove();
    uploadModal = null;
    fileQueue = [];
  };

  closeBtn.onclick = closeModal;
  cancelBtn.onclick = closeModal;

  clearBtn.onclick = () => {
    fileQueue = [];
    updateQueueUI();
  };

  // Files picker trigger
  selectFilesBtn.onclick = (e) => {
    e.stopPropagation();
    filesInput.click();
  };
  filesInput.onchange = (e) => {
    const files = Array.from(e.target.files).map(file => ({
      file,
      path: file.name
    }));
    appendFilesToQueue(files);
    filesInput.value = ''; // Reset
  };

  // Folder picker trigger
  selectFolderBtn.onclick = (e) => {
    e.stopPropagation();
    folderInput.click();
  };
  folderInput.onchange = (e) => {
    const files = Array.from(e.target.files).map(file => ({
      file,
      path: file.webkitRelativePath || file.name
    }));
    appendFilesToQueue(files);
    folderInput.value = ''; // Reset
  };

  // Dropzone drag-and-drop
  dropZone.onclick = (e) => {
    if (e.target !== selectFilesBtn && e.target !== selectFolderBtn) {
      filesInput.click();
    }
  };
  
  dropZone.ondragover = (e) => {
    e.preventDefault();
    dropZone.classList.add('border-accent', 'bg-accent/5');
  };
  
  dropZone.ondragleave = (e) => {
    e.preventDefault();
    dropZone.classList.remove('border-accent', 'bg-accent/5');
  };
  
  dropZone.ondrop = async (e) => {
    e.preventDefault();
    dropZone.classList.remove('border-accent', 'bg-accent/5');
    
    const items = e.dataTransfer.items;
    if (!items) return;

    const fileEntries = [];
    for (let i = 0; i < items.length; i++) {
      const entry = items[i].webkitGetAsEntry();
      if (entry) {
        fileEntries.push(entry);
      }
    }

    if (fileEntries.length > 0) {
      showToast('Processing dropped items...', 'info');
      const filesList = [];
      for (const entry of fileEntries) {
        const files = await getFilesFromEntry(entry);
        filesList.push(...files);
      }
      appendFilesToQueue(filesList);
    }
  };

  // Upload Trigger
  uploadStartBtn.onclick = () => {
    startMultipartUpload(libraryId);
  };

  updateQueueUI();
}

async function getFilesFromEntry(entry, path = '') {
  if (entry.isFile) {
    const file = await new Promise((resolve, reject) => entry.file(resolve, reject));
    return [{ file, path: path + file.name }];
  } else if (entry.isDirectory) {
    const dirReader = entry.createReader();
    const entries = await new Promise((resolve, reject) => {
      dirReader.readEntries(resolve, reject);
    });
    const filePromises = entries.map(childEntry => 
      getFilesFromEntry(childEntry, path + entry.name + '/')
    );
    const results = await Promise.all(filePromises);
    return results.flat();
  }
  return [];
}

function startMultipartUpload(libraryId) {
  if (fileQueue.length === 0) return;

  const targetFolderSelect = uploadModal.querySelector('#upload-target-folder');
  const folderId = targetFolderSelect ? targetFolderSelect.value : '';

  // Switch modal view to upload progress
  const bodyContainer = uploadModal.querySelector('#upload-modal-body');
  const footerContainer = uploadModal.querySelector('#upload-modal-footer');

  bodyContainer.innerHTML = `
    <div class="flex flex-col items-center justify-center py-8 px-4 space-y-4">
      <div class="animate-spin rounded-full h-12 w-12 border-b-2 border-accent"></div>
      <div class="text-center">
        <h4 class="text-sm font-semibold text-white">Uploading file(s)...</h4>
        <p id="upload-status-text" class="text-xs text-black-100 mt-1">Preparing multipart form data</p>
      </div>
      
      <!-- Progress Bar Wrapper -->
      <div class="w-full bg-black-500 rounded-full h-2.5 overflow-hidden">
        <div id="upload-progress-bar" class="bg-accent h-2.5 rounded-full transition-all duration-100" style="width: 0%"></div>
      </div>
      
      <!-- Upload statistics -->
      <div class="flex justify-between w-full text-xs text-black-100 font-mono">
        <span id="upload-progress-pct">0%</span>
        <span id="upload-progress-bytes">0 B / 0 B</span>
      </div>
    </div>
  `;

  footerContainer.innerHTML = `
    <button id="abort-upload-btn" class="bg-red-500/10 hover:bg-red-500/20 border border-red-500/30 text-red-400 px-4 py-2 rounded text-sm font-semibold transition-colors focus:outline-none">
      Cancel & Abort
    </button>
  `;

  const progressBar = uploadModal.querySelector('#upload-progress-bar');
  const progressPct = uploadModal.querySelector('#upload-progress-pct');
  const progressBytes = uploadModal.querySelector('#upload-progress-bytes');
  const statusText = uploadModal.querySelector('#upload-status-text');
  const abortBtn = uploadModal.querySelector('#abort-upload-btn');

  // Build FormData
  const formData = new FormData();
  formData.append('library', libraryId);
  if (folderId) {
    formData.append('folder', folderId);
  }

  let totalBytes = 0;
  fileQueue.forEach(item => {
    formData.append('files', item.file, item.path);
    totalBytes += item.file.size;
  });

  // Setup XHR request for tracking progress
  const xhr = new XMLHttpRequest();
  xhr.open('POST', resolvePath('/api/upload'));

  const token = localStorage.getItem('token');
  if (token) {
    xhr.setRequestHeader('Authorization', `Bearer ${token}`);
  }

  abortBtn.onclick = () => {
    xhr.abort();
    showToast('Upload aborted by user', 'warning');
    // Reload normal modal layout
    openUploadModal(libraryId, fileQueue);
  };

  // Progress listener
  let startTime = Date.now();
  xhr.upload.onprogress = (e) => {
    if (e.lengthComputable) {
      const pct = Math.round((e.loaded / e.total) * 100);
      progressBar.style.width = `${pct}%`;
      progressPct.textContent = `${pct}%`;
      progressBytes.textContent = `${formatBytes(e.loaded)} / ${formatBytes(e.total)}`;

      // Calculate speed and ETA
      const elapsed = (Date.now() - startTime) / 1000; // seconds
      if (elapsed > 0.5) {
        const speed = e.loaded / elapsed;
        const eta = (e.total - e.loaded) / speed;
        statusText.textContent = `Uploading at ${formatBytes(speed)}/s | ETA: ${Math.round(eta)}s`;
      } else {
        statusText.textContent = `Uploading...`;
      }
    }
  };

  xhr.onload = () => {
    if (xhr.status >= 200 && xhr.status < 300) {
      let resp = {};
      try {
        resp = JSON.parse(xhr.responseText);
      } catch (_) {}

      showToast(resp.message || 'Upload complete! Library scan triggered.', 'success');
      
      // Render success screen
      bodyContainer.innerHTML = `
        <div class="flex flex-col items-center justify-center py-8 px-4 text-center space-y-3">
          <span class="material-symbols text-5xl text-emerald-400 animate-bounce">check_circle</span>
          <h4 class="text-sm font-semibold text-white">Upload Completed Successfully</h4>
          <p class="text-xs text-black-100 max-w-sm">Uploaded ${fileQueue.length} files successfully. A database library scan is running in the background to discover the newly uploaded files.</p>
        </div>
      `;

      footerContainer.innerHTML = `
        <button id="upload-success-close-btn" class="bg-accent hover:bg-accent-hover text-primary px-5 py-2 rounded text-sm font-semibold transition-colors focus:outline-none">
          Done
        </button>
      `;

      footerContainer.querySelector('#upload-success-close-btn').onclick = () => {
        uploadModal.remove();
        uploadModal = null;
        fileQueue = [];
      };

    } else {
      let errText = xhr.responseText;
      try {
        const errObj = JSON.parse(xhr.responseText);
        errText = errObj.error || errText;
      } catch (_) {}
      
      showToast('Upload failed: ' + errText, 'error');
      openUploadModal(libraryId, fileQueue);
    }
  };

  xhr.onerror = () => {
    showToast('Network error during upload', 'error');
    openUploadModal(libraryId, fileQueue);
  };

  xhr.send(formData);
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
