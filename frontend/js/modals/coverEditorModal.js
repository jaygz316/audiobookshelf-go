import { request, resolvePath } from '../api.js';
import { escapeHtml } from '../itemDetails.js';

export function triggerCoverEditorModal(item, libraryId, onSaveSuccess) {
  const modal = document.createElement('div');
  modal.className = 'fixed inset-0 bg-black-900/80 backdrop-blur-sm z-50 flex items-center justify-center p-4 select-none';
  
  const mediaType = item.mediaType || 'book';
  const currentTitle = item.media?.metadata?.title || '';
  const currentAuthor = item.media?.metadata?.authorName || (item.media?.metadata?.authors && item.media?.metadata?.authors[0]?.name) || '';

  modal.innerHTML = `
    <div class="bg-primary border border-black-300 w-full max-w-4xl p-6 rounded-md shadow-2xl flex flex-col max-h-[90vh]">
      <!-- Header -->
      <div class="flex justify-between items-center border-b border-black-400 pb-3 flex-shrink-0">
        <h3 class="text-lg font-bold text-white flex items-center space-x-2">
          <span class="material-symbols text-accent">image_editor</span>
          <span>Cover Art Canvas Editor</span>
        </h3>
        <button id="close-editor-modal" class="text-black-100 hover:text-white transition-colors">
          <span class="material-symbols text-xl">close</span>
        </button>
      </div>

      <!-- Main Body -->
      <div class="grid grid-cols-1 md:grid-cols-2 gap-6 overflow-y-auto py-4 flex-grow no-scroll">
        <!-- Left: Canvas Area & Crop Controls -->
        <div class="flex flex-col space-y-4">
          <div class="border border-black-400 rounded bg-black-900 p-2 flex items-center justify-center h-[360px] relative overflow-hidden">
            <canvas id="editor-canvas" class="max-w-full max-h-full cursor-crosshair"></canvas>
            <div id="editor-empty-state" class="absolute inset-0 flex flex-col items-center justify-center text-black-200 text-xs pointer-events-none">
              <span class="material-symbols text-4xl mb-2">image</span>
              <span>No image loaded. Upload a file or search below.</span>
            </div>
          </div>

          <!-- Crop Controls -->
          <div class="bg-black-500/30 border border-black-400/50 rounded p-3 space-y-3">
            <div class="flex items-center justify-between text-xs text-black-100">
              <span class="font-semibold">Aspect Ratio:</span>
              <div class="flex space-x-2">
                <button id="ratio-free" class="px-2 py-1 rounded bg-accent text-primary font-bold">Free</button>
                <button id="ratio-1-1" class="px-2 py-1 rounded bg-black-400 hover:bg-black-300 text-white font-bold">1:1</button>
                <button id="ratio-2-3" class="px-2 py-1 rounded bg-black-400 hover:bg-black-300 text-white font-bold">2:3</button>
              </div>
            </div>
            
            <div class="flex space-x-2">
              <button id="apply-crop-btn" class="flex-1 bg-accent hover:opacity-90 disabled:opacity-50 text-primary font-bold py-1.5 px-3 rounded text-xs transition-all" disabled>
                Apply Crop
              </button>
              <button id="reset-canvas-btn" class="bg-black-400 hover:bg-black-300 text-white font-semibold py-1.5 px-3 rounded text-xs transition-all">
                Reset
              </button>
            </div>
          </div>
        </div>

        <!-- Right: Search, Upload & Background Fill Tabs -->
        <div class="flex flex-col h-full min-h-[300px]">
          <!-- Tab Headers -->
          <div class="flex border-b border-black-400 text-xs font-semibold mb-4 flex-shrink-0">
            <button id="tab-btn-search" class="px-4 py-2 border-b-2 border-accent text-white">Search Providers</button>
            <button id="tab-btn-upload" class="px-4 py-2 border-b-2 border-transparent text-black-100 hover:text-white">Upload File</button>
            <button id="tab-btn-bg" class="px-4 py-2 border-b-2 border-transparent text-black-100 hover:text-white">Padding & Color</button>
          </div>

          <!-- Tab Content Container -->
          <div class="flex-grow flex flex-col min-h-0 overflow-y-auto no-scroll">
            <!-- Tab: Search Providers -->
            <div id="editor-tab-search" class="space-y-4 flex flex-col flex-grow">
              <div class="grid grid-cols-1 sm:grid-cols-3 gap-2 flex-shrink-0">
                <select id="editor-provider" class="bg-black-500 text-white px-2 py-1.5 rounded border border-black-300 focus:outline-none focus:border-accent text-xs">
                  <option value="google-books">Google Books</option>
                  <option value="open-library">Open Library</option>
                  <option value="audible">Audible</option>
                  <option value="itunes">iTunes</option>
                </select>
                <input type="text" id="editor-search-query" placeholder="Search title/author..." class="bg-black-500 text-white px-2 py-1.5 rounded border border-black-300 focus:outline-none focus:border-accent text-xs sm:col-span-2">
              </div>
              <button id="editor-search-btn" class="w-full bg-accent hover:opacity-90 text-primary font-bold py-1.5 px-3 rounded text-xs flex items-center justify-center space-x-1 flex-shrink-0">
                <span class="material-symbols text-sm">search</span>
                <span>Search Provider</span>
              </button>

              <div id="editor-search-results" class="flex-grow border border-black-400/40 rounded p-2 bg-black-900/40 overflow-y-auto min-h-[150px] no-scroll">
                <p class="text-center text-xs text-black-200 py-8">Search results will display here.</p>
              </div>
            </div>

            <!-- Tab: Upload File -->
            <div id="editor-tab-upload" class="hidden space-y-4">
              <div id="editor-upload-zone" class="border-2 border-dashed border-black-400 hover:border-accent rounded-md p-8 flex flex-col items-center justify-center space-y-2 cursor-pointer transition-colors bg-black-500/10">
                <span class="material-symbols text-3xl text-black-100">upload_file</span>
                <span class="text-xs text-white font-medium">Drag & Drop Cover Image Here</span>
                <span class="text-[0.65rem] text-black-200">Supports PNG, JPG, JPEG, WEBP</span>
                <input type="file" id="editor-file-input" accept="image/*" class="hidden">
              </div>
            </div>

            <!-- Tab: Padding & Color -->
            <div id="editor-tab-bg" class="hidden space-y-4">
              <div class="bg-black-500/20 border border-black-400/50 rounded p-3 space-y-3 text-xs">
                <div>
                  <label class="block text-black-100 mb-1 font-semibold">Background Fill Color:</label>
                  <div class="flex items-center space-x-3">
                    <input type="color" id="editor-bg-color" value="#000000" class="bg-transparent border-0 w-8 h-8 cursor-pointer rounded">
                    <input type="text" id="editor-bg-color-hex" value="#000000" class="bg-black-500 text-white px-2 py-1 rounded border border-black-300 w-24 text-xs font-mono text-center">
                  </div>
                </div>
                
                <div class="space-y-2 pt-2">
                  <span class="block text-black-100 font-semibold">Predefined Palette:</span>
                  <div class="flex flex-wrap gap-2" id="editor-color-palette">
                    <!-- Palette items -->
                  </div>
                </div>

                <div class="pt-2">
                  <label class="block text-black-100 mb-1 font-semibold">Fit Canvas Margin Padding (px):</label>
                  <div class="flex items-center space-x-3">
                    <input type="range" id="editor-padding-slider" min="0" max="100" value="0" class="w-full accent-accent">
                    <span id="editor-padding-val" class="font-mono text-xs w-8 text-right">0px</span>
                  </div>
                </div>

                <div class="pt-2">
                  <button id="editor-apply-bg-btn" class="w-full bg-accent hover:opacity-90 text-primary font-bold py-1.5 px-3 rounded text-xs">
                    Apply Padding & Background Color
                  </button>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- Footer Buttons -->
      <div class="flex justify-end space-x-3 pt-3 border-t border-black-400 flex-shrink-0">
        <button id="close-editor-modal-cancel" class="bg-black-400 hover:bg-black-300 text-white px-4 py-2 rounded text-xs font-semibold">
          Cancel
        </button>
        <button id="save-editor-cover-btn" class="bg-accent hover:opacity-90 disabled:opacity-50 text-primary font-bold px-5 py-2 rounded text-xs transition-opacity shadow" disabled>
          Save Cover
        </button>
      </div>
    </div>
  `;

  document.body.appendChild(modal);

  const closeModal = () => {
    window.onmousemove = null;
    window.onmouseup = null;
    modal.remove();
  };
  document.getElementById('close-editor-modal').onclick = closeModal;
  document.getElementById('close-editor-modal-cancel').onclick = closeModal;

  const tabBtnSearch = document.getElementById('tab-btn-search');
  const tabBtnUpload = document.getElementById('tab-btn-upload');
  const tabBtnBg = document.getElementById('tab-btn-bg');

  const tabSearch = document.getElementById('editor-tab-search');
  const tabUpload = document.getElementById('editor-tab-upload');
  const tabBg = document.getElementById('editor-tab-bg');

  const switchTab = (activeBtn, activeTab) => {
    [tabBtnSearch, tabBtnUpload, tabBtnBg].forEach(btn => {
      btn.classList.remove('border-accent', 'text-white');
      btn.classList.add('border-transparent', 'text-black-100');
    });
    activeBtn.classList.remove('border-transparent', 'text-black-100');
    activeBtn.classList.add('border-accent', 'text-white');

    [tabSearch, tabUpload, tabBg].forEach(t => t.classList.add('hidden'));
    activeTab.classList.remove('hidden');
  };

  tabBtnSearch.onclick = () => switchTab(tabBtnSearch, tabSearch);
  tabBtnUpload.onclick = () => switchTab(tabBtnUpload, tabUpload);
  tabBtnBg.onclick = () => switchTab(tabBtnBg, tabBg);

  const providerSelect = document.getElementById('editor-provider');
  const searchQuery = document.getElementById('editor-search-query');
  const searchBtn = document.getElementById('editor-search-btn');
  const resultsContainer = document.getElementById('editor-search-results');

  searchQuery.value = `${currentTitle} ${currentAuthor}`.trim();

  request('GET', '/api/search/providers')
    .then(data => {
      const providers = data.providers?.booksCovers || [];
      if (providers.length > 0) {
        providerSelect.innerHTML = providers.map(p => `<option value="${p.value}">${escapeHtml(p.text)}</option>`).join('');
        if (providers.some(p => p.value === 'google')) {
          providerSelect.value = 'google';
        }
      }
    })
    .catch(err => {
      console.error('Failed to load search providers:', err);
    });

  searchBtn.onclick = async () => {
    const provider = providerSelect.value;
    const q = searchQuery.value.trim();
    if (!q) return;

    resultsContainer.innerHTML = `
      <div class="flex items-center justify-center py-8">
        <div class="animate-spin rounded-full h-8 w-8 border-b-2 border-accent"></div>
      </div>
    `;

    try {
      let results = [];
      if (mediaType === 'book') {
        const queryParams = new URLSearchParams({ provider, title: q });
        results = await request('GET', `/api/search/books?${queryParams.toString()}`);
      } else {
        const queryParams = new URLSearchParams({ term: q });
        results = await request('GET', `/api/search/podcast?${queryParams.toString()}`);
      }

      if (!results || results.length === 0) {
        resultsContainer.innerHTML = `<p class="text-xs text-black-100 text-center py-6">No results found.</p>`;
        return;
      }

      resultsContainer.innerHTML = `
        <div class="grid grid-cols-3 gap-2">
          ${results.map((res, idx) => {
            const coverUrl = res.coverUrl;
            if (!coverUrl) return '';
            return `
              <div class="editor-search-result-item border border-black-400 hover:border-accent rounded overflow-hidden cursor-pointer bg-black-900 relative group aspect-[2/3]" data-idx="${idx}">
                <img src="${escapeHtml(coverUrl)}" class="w-full h-full object-cover" alt="">
                <div class="absolute inset-0 bg-black-950/70 opacity-0 group-hover:opacity-100 flex items-center justify-center transition-opacity p-1 text-[0.65rem] text-white text-center font-semibold">
                  Select
                </div>
              </div>
            `;
          }).join('')}
        </div>
      `;

      resultsContainer.querySelectorAll('.editor-search-result-item').forEach(el => {
        el.onclick = async () => {
          const idx = parseInt(el.getAttribute('data-idx'), 10);
          const res = results[idx];
          
          const overlay = el.querySelector('div');
          const origText = overlay.textContent;
          overlay.textContent = 'Loading...';
          overlay.style.opacity = 1;

          try {
            await request('POST', `/api/items/${item.id}/cover-from-url`, { coverUrl: res.coverUrl });
            const ts = Date.now();
            const token = localStorage.getItem('token') || '';
            const localCoverUrl = resolvePath(`/api/items/${item.id}/cover?raw=1&token=${token}&ts=${ts}`);
            initEditor(localCoverUrl);
          } catch (err) {
            showToast('Failed to fetch cover from provider: ' + err.message, 'error');
          } finally {
            overlay.textContent = origText;
            overlay.style.opacity = '';
          }
        };
      });
    } catch (err) {
      resultsContainer.innerHTML = `<p class="text-xs text-red-400 text-center py-6">Failed: ${escapeHtml(err.message)}</p>`;
    }
  };

  const uploadZone = document.getElementById('editor-upload-zone');
  const fileInput = document.getElementById('editor-file-input');

  uploadZone.onclick = () => fileInput.click();

  fileInput.onchange = (e) => {
    const file = e.target.files[0];
    if (file) {
      const reader = new FileReader();
      reader.onload = (evt) => {
        initEditor(evt.target.result);
      };
      reader.readAsDataURL(file);
    }
  };

  uploadZone.ondragover = (e) => {
    e.preventDefault();
    uploadZone.classList.add('border-accent', 'bg-black-500/20');
  };

  uploadZone.ondragleave = () => {
    uploadZone.classList.remove('border-accent', 'bg-black-500/20');
  };

  uploadZone.ondrop = (e) => {
    e.preventDefault();
    uploadZone.classList.remove('border-accent', 'bg-black-500/20');
    const file = e.dataTransfer.files[0];
    if (file && file.type.startsWith('image/')) {
      const reader = new FileReader();
      reader.onload = (evt) => {
        initEditor(evt.target.result);
      };
      reader.readAsDataURL(file);
    }
  };

  const bgColorInput = document.getElementById('editor-bg-color');
  const bgColorHex = document.getElementById('editor-bg-color-hex');
  const paddingSlider = document.getElementById('editor-padding-slider');
  const paddingVal = document.getElementById('editor-padding-val');
  const applyBgBtn = document.getElementById('editor-apply-bg-btn');
  const colorPalette = document.getElementById('editor-color-palette');

  let bgColor = '#000000';

  const updateBgColor = (hex) => {
    bgColor = hex;
    bgColorInput.value = hex;
    bgColorHex.value = hex;
  };

  bgColorInput.oninput = (e) => updateBgColor(e.target.value);
  bgColorHex.oninput = (e) => {
    const val = e.target.value;
    if (val.match(/^#[0-9A-Fa-f]{6}$/)) {
      updateBgColor(val);
    }
  };

  paddingSlider.oninput = (e) => {
    paddingVal.textContent = `${e.target.value}px`;
  };

  const colors = ['#000000', '#ffffff', '#1a202c', '#742a2a', '#2b6cb0', '#2f855a', '#d69e2e', '#4a5568'];
  colorPalette.innerHTML = colors.map(c => `
    <button class="w-6 h-6 rounded-full border border-black-300 transition-transform hover:scale-110" style="background-color: ${c}" data-color="${c}"></button>
  `).join('');

  colorPalette.querySelectorAll('button').forEach(btn => {
    btn.onclick = () => updateBgColor(btn.getAttribute('data-color'));
  });

  applyBgBtn.onclick = () => {
    if (!originalImg.complete || !originalImg.src) return;
    const pad = parseInt(paddingSlider.value, 10);
    
    const tempCanvas = document.createElement('canvas');
    tempCanvas.width = originalImg.width + pad * 2;
    tempCanvas.height = originalImg.height + pad * 2;
    const tempCtx = tempCanvas.getContext('2d');
    
    tempCtx.fillStyle = bgColor;
    tempCtx.fillRect(0, 0, tempCanvas.width, tempCanvas.height);
    tempCtx.drawImage(originalImg, pad, pad);
    
    initEditor(tempCanvas.toDataURL());
  };

  let originalImg = new Image();
  let canvas = document.getElementById('editor-canvas');
  let ctx = canvas.getContext('2d');
  let cropBox = { x: 0, y: 0, w: 0, h: 0 };
  let isDragging = false;
  let dragOffset = { x: 0, y: 0 };
  let imgScale = 1;
  let activeHandle = null;
  let aspectRatio = 'free';
  let historyStack = [];

  const updateAspectButtons = () => {
    ['ratio-free', 'ratio-1-1', 'ratio-2-3'].forEach(id => {
      const btn = document.getElementById(id);
      if (id === `ratio-${aspectRatio.replace(':', '-')}`) {
        btn.className = 'px-2 py-1 rounded bg-accent text-primary font-bold text-xs';
      } else {
        btn.className = 'px-2 py-1 rounded bg-black-400 hover:bg-black-300 text-white font-bold text-xs';
      }
    });
  };

  document.getElementById('ratio-free').onclick = () => {
    aspectRatio = 'free';
    updateAspectButtons();
    resetCropBox();
  };
  document.getElementById('ratio-1-1').onclick = () => {
    aspectRatio = '1:1';
    updateAspectButtons();
    resetCropBox();
  };
  document.getElementById('ratio-2-3').onclick = () => {
    aspectRatio = '2:3';
    updateAspectButtons();
    resetCropBox();
  };

  const resetCropBox = () => {
    if (!canvas || !originalImg.complete) return;
    cropBox.w = canvas.width * 0.8;
    if (aspectRatio === '1:1') {
      cropBox.h = cropBox.w;
    } else if (aspectRatio === '2:3') {
      cropBox.h = (cropBox.w * 3) / 2;
      if (cropBox.h > canvas.height * 0.8) {
        cropBox.h = canvas.height * 0.8;
        cropBox.w = (cropBox.h * 2) / 3;
      }
    } else {
      cropBox.h = canvas.height * 0.8;
    }
    cropBox.x = (canvas.width - cropBox.w) / 2;
    cropBox.y = (canvas.height - cropBox.h) / 2;
    draw();
  };

  const initEditor = (src) => {
    if (!src) return;
    if (originalImg.src) {
      historyStack.push(originalImg.src);
    }

    originalImg.onload = () => {
      document.getElementById('save-editor-cover-btn').disabled = false;
      document.getElementById('apply-crop-btn').disabled = false;
      document.getElementById('editor-empty-state').classList.add('hidden');
      
      const parent = canvas.parentElement;
      const parentWidth = parent.clientWidth - 16;
      const parentHeight = parent.clientHeight - 16;
      
      const scaleX = parentWidth / originalImg.width;
      const scaleY = parentHeight / originalImg.height;
      imgScale = Math.min(scaleX, scaleY, 1);
      
      canvas.width = originalImg.width * imgScale;
      canvas.height = originalImg.height * imgScale;
      
      resetCropBox();
    };
    originalImg.crossOrigin = 'anonymous';
    originalImg.src = src;
  };

  const draw = () => {
    if (!ctx || !originalImg.complete) return;
    
    ctx.clearRect(0, 0, canvas.width, canvas.height);
    ctx.drawImage(originalImg, 0, 0, canvas.width, canvas.height);
    
    ctx.fillStyle = 'rgba(0, 0, 0, 0.6)';
    ctx.fillRect(0, 0, canvas.width, cropBox.y);
    ctx.fillRect(0, cropBox.y + cropBox.h, canvas.width, canvas.height - (cropBox.y + cropBox.h));
    ctx.fillRect(0, cropBox.y, cropBox.x, cropBox.h);
    ctx.fillRect(cropBox.x + cropBox.w, cropBox.y, canvas.width - (cropBox.x + cropBox.w), cropBox.h);
    
    ctx.strokeStyle = '#e5a93c';
    ctx.lineWidth = 2;
    ctx.strokeRect(cropBox.x, cropBox.y, cropBox.w, cropBox.h);
    
    ctx.fillStyle = '#ffffff';
    const handleSize = 8;
    const hs = handleSize / 2;
    
    ctx.fillRect(cropBox.x - hs, cropBox.y - hs, handleSize, handleSize);
    ctx.fillRect(cropBox.x + cropBox.w - hs, cropBox.y - hs, handleSize, handleSize);
    ctx.fillRect(cropBox.x - hs, cropBox.y + cropBox.h - hs, handleSize, handleSize);
    ctx.fillRect(cropBox.x + cropBox.w - hs, cropBox.y + cropBox.h - hs, handleSize, handleSize);
  };

  const getHandleAt = (mx, my) => {
    const handleSize = 16;
    const hs = handleSize / 2;
    
    if (Math.abs(mx - cropBox.x) < hs && Math.abs(my - cropBox.y) < hs) return 'tl';
    if (Math.abs(mx - (cropBox.x + cropBox.w)) < hs && Math.abs(my - cropBox.y) < hs) return 'tr';
    if (Math.abs(mx - cropBox.x) < hs && Math.abs(my - (cropBox.y + cropBox.h)) < hs) return 'bl';
    if (Math.abs(mx - (cropBox.x + cropBox.w)) < hs && Math.abs(my - (cropBox.y + cropBox.h)) < hs) return 'br';
    
    if (mx >= cropBox.x && mx <= cropBox.x + cropBox.w && my >= cropBox.y && my <= cropBox.y + cropBox.h) {
      return 'drag';
    }
    return null;
  };

  canvas.onmousedown = (e) => {
    if (!originalImg.complete || !originalImg.src) return;
    const rect = canvas.getBoundingClientRect();
    const mx = e.clientX - rect.left;
    const my = e.clientY - rect.top;
    
    activeHandle = getHandleAt(mx, my);
    if (activeHandle) {
      isDragging = true;
      dragOffset.x = mx - (activeHandle === 'drag' ? cropBox.x : 0);
      dragOffset.y = my - (activeHandle === 'drag' ? cropBox.y : 0);
    }
  };

  window.onmousemove = (e) => {
    if (!isDragging || !canvas) return;
    const rect = canvas.getBoundingClientRect();
    const mx = e.clientX - rect.left;
    const my = e.clientY - rect.top;
    
    if (activeHandle === 'drag') {
      let nx = mx - dragOffset.x;
      let ny = my - dragOffset.y;
      
      nx = Math.max(0, Math.min(canvas.width - cropBox.w, nx));
      ny = Math.max(0, Math.min(canvas.height - cropBox.h, ny));
      
      cropBox.x = nx;
      cropBox.y = ny;
    } else {
      const minSize = 20;
      let nx = cropBox.x;
      let ny = cropBox.y;
      let nw = cropBox.w;
      let nh = cropBox.h;
      
      if (activeHandle === 'tl') {
        nx = Math.max(0, Math.min(cropBox.x + cropBox.w - minSize, mx));
        ny = Math.max(0, Math.min(cropBox.y + cropBox.h - minSize, my));
        nw = cropBox.w + (cropBox.x - nx);
        nh = cropBox.h + (cropBox.y - ny);
      } else if (activeHandle === 'tr') {
        nw = Math.max(minSize, Math.min(canvas.width - cropBox.x, mx - cropBox.x));
        ny = Math.max(0, Math.min(cropBox.y + cropBox.h - minSize, my));
        nh = cropBox.h + (cropBox.y - ny);
      } else if (activeHandle === 'bl') {
        nx = Math.max(0, Math.min(cropBox.x + cropBox.w - minSize, mx));
        nw = cropBox.w + (cropBox.x - nx);
        nh = Math.max(minSize, Math.min(canvas.height - cropBox.y, my - cropBox.y));
      } else if (activeHandle === 'br') {
        nw = Math.max(minSize, Math.min(canvas.width - cropBox.x, mx - cropBox.x));
        nh = Math.max(minSize, Math.min(canvas.height - cropBox.y, my - cropBox.y));
      }
      
      if (aspectRatio === '1:1') {
        const size = Math.min(nw, nh);
        nw = size;
        nh = size;
        if (activeHandle === 'tl') {
          nx = cropBox.x + cropBox.w - nw;
          ny = cropBox.y + cropBox.h - nh;
        } else if (activeHandle === 'tr') {
          ny = cropBox.y + cropBox.h - nh;
        } else if (activeHandle === 'bl') {
          nx = cropBox.x + cropBox.w - nw;
        }
      } else if (aspectRatio === '2:3') {
        let targetH = nw * 1.5;
        if (targetH > canvas.height - (activeHandle.includes('t') ? 0 : cropBox.y)) {
          targetH = canvas.height - (activeHandle.includes('t') ? 0 : cropBox.y);
          nw = targetH / 1.5;
        }
        nh = targetH;
        
        if (activeHandle === 'tl') {
          nx = cropBox.x + cropBox.w - nw;
          ny = cropBox.y + cropBox.h - nh;
        } else if (activeHandle === 'tr') {
          ny = cropBox.y + cropBox.h - nh;
        } else if (activeHandle === 'bl') {
          nx = cropBox.x + cropBox.w - nw;
        }
      }
      
      cropBox.x = nx;
      cropBox.y = ny;
      cropBox.w = nw;
      cropBox.h = nh;
    }
    draw();
  };

  window.onmouseup = () => {
    isDragging = false;
    activeHandle = null;
  };

  canvas.ontouchstart = (e) => {
    if (e.touches.length === 1) {
      const fakeEvent = {
        clientX: e.touches[0].clientX,
        clientY: e.touches[0].clientY
      };
      canvas.onmousedown(fakeEvent);
    }
  };
  canvas.ontouchmove = (e) => {
    if (e.touches.length === 1) {
      const fakeEvent = {
        clientX: e.touches[0].clientX,
        clientY: e.touches[0].clientY
      };
      window.onmousemove(fakeEvent);
      e.preventDefault();
    }
  };
  canvas.ontouchend = () => {
    window.onmouseup();
  };

  document.getElementById('apply-crop-btn').onclick = () => {
    if (!originalImg.complete || !originalImg.src) return;
    
    const rx = cropBox.x / imgScale;
    const ry = cropBox.y / imgScale;
    const rw = cropBox.w / imgScale;
    const rh = cropBox.h / imgScale;
    
    const tempCanvas = document.createElement('canvas');
    tempCanvas.width = rw;
    tempCanvas.height = rh;
    const tempCtx = tempCanvas.getContext('2d');
    
    tempCtx.drawImage(originalImg, rx, ry, rw, rh, 0, 0, rw, rh);
    
    initEditor(tempCanvas.toDataURL());
  };

  document.getElementById('reset-canvas-btn').onclick = () => {
    if (historyStack.length > 0) {
      const initialSrc = historyStack[0];
      historyStack = [];
      initEditor(initialSrc);
    }
  };

  document.getElementById('save-editor-cover-btn').onclick = () => {
    if (!originalImg.complete || !originalImg.src) return;
    
    const tempCanvas = document.createElement('canvas');
    tempCanvas.width = originalImg.width;
    tempCanvas.height = originalImg.height;
    const tempCtx = tempCanvas.getContext('2d');
    tempCtx.drawImage(originalImg, 0, 0);
    
    tempCanvas.toBlob(async (blob) => {
      const formData = new FormData();
      formData.append('cover', blob, 'cover.jpg');
      
      try {
        const saveBtn = document.getElementById('save-editor-cover-btn');
        saveBtn.disabled = true;
        saveBtn.textContent = 'Saving...';
        
        const response = await fetch(resolvePath(`/api/items/${item.id}/cover`), {
          method: 'POST',
          headers: {
            'Authorization': `Bearer ${localStorage.getItem('token') || ''}`
          },
          body: formData
        });
        
        if (!response.ok) {
          throw new Error(await response.text());
        }
        
        closeModal();
        if (onSaveSuccess) onSaveSuccess();
      } catch (err) {
        showToast('Failed to save cover: ' + err.message, 'error');
        document.getElementById('save-editor-cover-btn').disabled = false;
        document.getElementById('save-editor-cover-btn').textContent = 'Save Cover';
      }
    }, 'image/jpeg', 0.95);
  };

  const ts = Date.now();
  const token = localStorage.getItem('token') || '';
  const initialCoverUrl = resolvePath(`/api/items/${item.id}/cover?raw=1&token=${token}&ts=${ts}`);
  const imgCheck = new Image();
  imgCheck.onload = () => initEditor(initialCoverUrl);
  imgCheck.src = initialCoverUrl;
}
