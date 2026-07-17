import { request, resolvePath } from '../api.js';

export function triggerShareLinkModal(item) {
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
              <button id="delete-share-link-btn" class="w-full bg-red-900/40 hover:bg-red-900/60 border border-red-500/30 text-error hover:text-white hover:border-red-500/50 font-bold py-2 rounded text-xs flex items-center justify-center space-x-1.5 transition-colors cursor-pointer">
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
            showToast('Share link copied to clipboard!', 'success');
          } catch (err) {
            showToast('Failed to copy share link: ' + err.message, 'error');
          }
        };

        body.querySelector('#delete-share-link-btn').onclick = async () => {
          try {
            await request('DELETE', `/api/share/mediaitem/${itemShare.id}`);
            showToast('Share link removed successfully', 'success');
            checkAndRender();
          } catch (err) {
            showToast('Failed to delete share link: ' + err.message, 'error');
          }
        };

      } else {
        body.innerHTML = `
          <form id="create-share-form" class="space-y-4">
            <div>
              <label class="block text-[0.7rem] uppercase font-semibold text-black-100 mb-1.5 font-bold">Expires In</label>
              <select id="share-duration" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent text-xs">
                <option value="3600000">1 Hour</option>
                <option value="86400000" selected>1 Day</option>
                <option value="604800000">7 Days</option>
                <option value="2592000000">30 Days</option>
                <option value="0">Never</option>
                <option value="custom">Custom Date & Time...</option>
              </select>
            </div>

            <div id="share-custom-expires-container" class="hidden">
              <label class="block text-[0.7rem] uppercase font-semibold text-black-100 mb-1.5 font-bold">Custom Expiration Date & Time</label>
              <input type="datetime-local" id="share-custom-expires" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent text-xs">
            </div>

            <div>
              <label class="block text-[0.7rem] uppercase font-semibold text-black-100 mb-1.5 font-bold">Max Downloads (0 for unlimited)</label>
              <input type="number" id="share-max-downloads" min="0" value="0" class="w-full bg-black-500 text-white px-3 py-2 rounded border border-black-300 focus:outline-none focus:border-accent text-xs">
            </div>

            <div class="flex items-center space-x-2">
              <input type="checkbox" id="share-allow-download" checked class="rounded border-black-300 text-accent focus:ring-accent bg-black-500 h-4 w-4">
              <label for="share-allow-download" class="text-xs font-medium text-white">Allow downloads</label>
            </div>

            <div class="flex items-center space-x-2">
              <input type="checkbox" id="share-embeddable" class="rounded border-black-300 text-accent focus:ring-accent bg-black-500 h-4 w-4">
              <label for="share-embeddable" class="text-xs font-medium text-white">Enable embeddable mini-player layout</label>
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

        const durationSelect = body.querySelector('#share-duration');
        const customExpiresContainer = body.querySelector('#share-custom-expires-container');
        const customExpiresInput = body.querySelector('#share-custom-expires');

        durationSelect.onchange = () => {
          if (durationSelect.value === 'custom') {
            customExpiresContainer.classList.remove('hidden');
            customExpiresInput.required = true;
          } else {
            customExpiresContainer.classList.add('hidden');
            customExpiresInput.required = false;
            customExpiresInput.value = '';
          }
        };

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
          
          const durationVal = durationSelect.value;
          const isDownloadable = body.querySelector('#share-allow-download').checked;
          const embeddableVal = body.querySelector('#share-embeddable').checked;
          const maxDownloadsVal = parseInt(body.querySelector('#share-max-downloads').value, 10) || 0;
          const passwordVal = passwordInput.value;

          const slug = generateSlug();
          let expiresAt = 0;
          if (durationVal === 'custom') {
            const customVal = customExpiresInput.value;
            if (customVal) {
              expiresAt = new Date(customVal).getTime();
            }
          } else {
            const durationMs = parseInt(durationVal, 10);
            if (durationMs > 0) {
              expiresAt = Date.now() + durationMs;
            }
          }

          try {
            await request('POST', '/api/share/mediaitem', {
              slug: slug,
              mediaItemId: item.id,
              mediaItemType: item.mediaType,
              expiresAt: expiresAt,
              isDownloadable: isDownloadable,
              password: passwordVal,
              maxDownloads: maxDownloadsVal,
              embeddable: embeddableVal
            });

            showToast('Share link generated successfully!', 'success');
            checkAndRender();
          } catch (err) {
            showToast('Failed to generate share link: ' + err.message, 'error');
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
