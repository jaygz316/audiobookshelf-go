export function showToast(message, type = 'info') {
  const container = document.getElementById('toast-container');
  if (!container) return;
  
  const toast = document.createElement('div');
  toast.className = 'px-4 py-2.5 rounded shadow-lg text-sm transition-all duration-300 transform translate-y-2 opacity-0 flex items-center space-x-2 ';
  
  if (type === 'success') {
    toast.className += 'bg-emerald-800 border border-emerald-500 text-emerald-100';
  } else if (type === 'error') {
    toast.className += 'bg-red-950 border border-red-500 text-red-100';
  } else if (type === 'warning') {
    toast.className += 'bg-yellow-950/80 border border-yellow-600/50 text-yellow-100';
  } else {
    toast.className += 'bg-primary border border-black-300 text-white';
  }
  
  const iconName = type === 'success' ? 'check_circle' : type === 'error' ? 'error' : type === 'warning' ? 'warning' : 'info';
  toast.innerHTML = `
    <span class="material-symbols text-lg">${iconName}</span>
    <span>${message}</span>
  `;
  
  container.appendChild(toast);
  
  setTimeout(() => {
    toast.classList.remove('translate-y-2', 'opacity-0');
  }, 10);
  
  setTimeout(() => {
    toast.classList.add('translate-y-2', 'opacity-0');
    setTimeout(() => {
      toast.remove();
    }, 300);
  }, 4000);
}

window.showToast = showToast;

export function showConfirm(title, message, confirmText = 'Confirm', cancelText = 'Cancel') {
  return new Promise((resolve) => {
    const modal = document.createElement('div');
    modal.className = 'fixed inset-0 bg-black-900/80 z-[100] flex items-center justify-center p-4 overflow-y-auto animate-fade-in';
    
    const escapeHtml = (str) => {
      if (!str) return '';
      return String(str)
        .replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;')
        .replace(/"/g, '&quot;')
        .replace(/'/g, '&#039;');
    };

    modal.innerHTML = `
      <div class="bg-primary border border-black-300 w-full max-w-sm p-6 rounded-md shadow-lg space-y-4 my-8 relative transform scale-95 transition-transform duration-200" id="confirm-modal-box">
        <h3 class="text-md font-bold text-white border-b border-black-400 pb-2">${escapeHtml(title)}</h3>
        <p class="text-sm text-black-50 leading-relaxed">${escapeHtml(message)}</p>
        <div class="flex justify-end space-x-3 pt-2">
          <button type="button" id="confirm-cancel-btn" class="bg-black-400 hover:bg-black-300 text-white px-4 py-2 rounded text-xs font-semibold transition-colors flex items-center space-x-1 focus:outline-none">
            <span>${escapeHtml(cancelText)}</span>
          </button>
          <button type="button" id="confirm-ok-btn" class="bg-accent hover:opacity-90 text-primary font-bold px-4 py-2 rounded text-xs transition-opacity flex items-center space-x-1 focus:outline-none">
            <span>${escapeHtml(confirmText)}</span>
          </button>
        </div>
      </div>
    `;

    document.body.appendChild(modal);

    // Minor delay to trigger transition scale
    setTimeout(() => {
      const box = modal.querySelector('#confirm-modal-box');
      if (box) {
        box.classList.remove('scale-95');
        box.classList.add('scale-100');
      }
    }, 10);

    const cleanup = (value) => {
      const box = modal.querySelector('#confirm-modal-box');
      if (box) {
        box.classList.remove('scale-100');
        box.classList.add('scale-95');
      }
      modal.classList.add('opacity-0');
      setTimeout(() => {
        modal.remove();
        resolve(value);
      }, 150);
    };

    modal.querySelector('#confirm-cancel-btn').onclick = () => cleanup(false);
    modal.querySelector('#confirm-ok-btn').onclick = () => cleanup(true);
    
    // Close on clicking backdrop
    modal.onclick = (e) => {
      if (e.target === modal) cleanup(false);
    };
  });
}

window.showConfirm = showConfirm;

