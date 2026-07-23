import { request } from '../api.js';
import { showToast } from '../toast.js';
import { getLibrariesList } from '../library.js';

function escapeHtml(str) {
  if (!str) return '';
  return str
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#039;');
}

export async function renderUsersTab() {
  const container = document.getElementById('tab-users');
  if (!container) return;

  container.innerHTML = `<div class="animate-spin rounded-full h-8 w-8 border-b-2 border-accent mx-auto"></div>`;

  try {
    const [usersRes, currentUser] = await Promise.all([
      request('GET', '/api/users'),
      request('GET', '/api/me')
    ]);
    const users = usersRes.users || [];

    container.innerHTML = `
      <div class="bg-primary border border-black-300 p-6 rounded-md space-y-6">
        <div class="flex justify-between items-center border-b border-black-400 pb-4">
          <div>
            <h3 class="text-lg font-semibold">Users</h3>
            <p class="text-xs text-black-100 mt-1">Manage user accounts, authentication types, and access permissions.</p>
          </div>
          <button id="add-user-btn" class="bg-accent hover:opacity-90 text-primary font-bold px-4 py-2 rounded shadow-sm transition-opacity flex items-center space-x-1.5 text-sm">
            <span class="material-symbols text-lg">person_add</span>
            <span>Add User</span>
          </button>
        </div>

        <div class="overflow-x-auto">
          <table class="w-full text-left text-sm text-black-50">
            <thead>
              <tr class="border-b border-black-400/60 text-xs text-black-100 uppercase tracking-wider font-semibold">
                <th class="px-4 py-3">Username</th>
                <th class="px-4 py-3">Account Type</th>
                <th class="px-4 py-3">Last Seen</th>
                <th class="px-4 py-3">Created At</th>
                <th class="px-4 py-3 text-right">Actions</th>
              </tr>
            </thead>
            <tbody id="users-list-rows" class="divide-y divide-black-400/50"></tbody>
          </table>
          <div id="users-empty-state" class="text-center py-6 text-black-100 hidden">No users found.</div>
        </div>
      </div>
    `;

    renderUsersListRows(users, currentUser);

    document.getElementById('add-user-btn').onclick = () => {
      triggerUserModal(null, currentUser, () => renderUsersTab());
    };

  } catch (err) {
    container.innerHTML = `<p class="text-error text-sm">Failed to load users: ${err.message}</p>`;
  }
}

function renderUsersListRows(users, currentUser) {
  const tbody = document.getElementById('users-list-rows');
  const emptyState = document.getElementById('users-empty-state');
  if (!tbody) return;

  tbody.innerHTML = '';
  if (users.length === 0) {
    emptyState.classList.remove('hidden');
    return;
  }
  emptyState.classList.add('hidden');

  users.forEach(u => {
    const tr = document.createElement('tr');
    tr.className = 'hover:bg-black-500/30';

    const lastSeenFormatted = u.lastSeen ? window.formatDateTime(u.lastSeen) : 'Never';
    const createdAtFormatted = u.createdAt ? window.formatDateTime(u.createdAt) : 'Unknown';
    
    let typeDisplay = u.type || 'user';
    if (typeDisplay === 'root') typeDisplay = 'Root Admin';
    else if (typeDisplay === 'admin') typeDisplay = 'Admin';
    else if (typeDisplay === 'guest') typeDisplay = 'Guest';
    else typeDisplay = 'User';

    const canDelete = u.type !== 'root' && u.id !== currentUser.id;
    const canEdit = currentUser.type === 'root' || (currentUser.type === 'admin' && u.type !== 'root');

    tr.innerHTML = `
      <td class="px-4 py-3 font-semibold text-white flex items-center space-x-2">
        <span>${escapeHtml(u.username)}</span>
        ${u.isActive ? '<span class="bg-success/10 text-success border border-success/30 text-[10px] px-1.5 py-0.5 rounded font-semibold uppercase">Active</span>' : '<span class="bg-error/10 text-error border border-error/30 text-[10px] px-1.5 py-0.5 rounded font-semibold uppercase">Inactive</span>'}
        ${u.hasOpenIDLink ? '<span class="bg-accent/10 text-accent border border-accent/30 text-[10px] px-1.5 py-0.5 rounded font-semibold uppercase">OIDC</span>' : ''}
      </td>
      <td class="px-4 py-3 text-xs capitalize">${typeDisplay}</td>
      <td class="px-4 py-3 text-xs text-black-100">${lastSeenFormatted}</td>
      <td class="px-4 py-3 text-xs text-black-100">${createdAtFormatted}</td>
      <td class="px-4 py-3 text-right space-x-2">
        ${u.hasOpenIDLink && canEdit ? '<button class="unlink-oidc-btn bg-warning/20 hover:bg-warning/30 border border-warning/30 text-warning hover:text-white hover:border-warning/50 text-xs font-semibold px-2.5 py-1 rounded inline-flex items-center space-x-1 transition-colors cursor-pointer" data-id="' + u.id + '"><span class="material-symbols text-sm">link_off</span><span>Unlink</span></button>' : ''}
        <button class="edit-user-btn bg-accent/20 hover:bg-accent/30 border border-accent/30 hover:border-accent/50 text-accent hover:text-white text-xs font-bold px-3 py-1.5 rounded shadow-sm disabled:opacity-40 disabled:cursor-not-allowed inline-flex items-center space-x-1 transition-colors cursor-pointer" ${canEdit ? '' : 'disabled'} data-id="${u.id}">
          <span class="material-symbols text-sm">edit</span>
          <span>Edit</span>
        </button>
        <button class="delete-user-btn bg-red-900/40 hover:bg-red-900/60 border border-red-500/30 text-error hover:text-white hover:border-red-500/50 text-xs font-bold px-3 py-1.5 rounded shadow-sm disabled:opacity-40 disabled:cursor-not-allowed inline-flex items-center space-x-1 transition-colors cursor-pointer" ${canDelete ? '' : 'disabled'} data-id="${u.id}">
          <span class="material-symbols text-sm">delete</span>
          <span>Delete</span>
        </button>
      </td>
    `;

    if (canEdit) {
      tr.querySelector('.edit-user-btn').onclick = () => {
        triggerUserModal(u, currentUser, () => renderUsersTab());
      };
      if (u.hasOpenIDLink) {
        tr.querySelector('.unlink-oidc-btn').onclick = async () => {
          const confirmed = await window.showConfirm(
            'Unlink OIDC',
            `Are you sure you want to unlink OIDC for user "${u.username}"?`,
            'Unlink',
            'Cancel'
          );
          if (!confirmed) return;
          try {
            await request('PATCH', `/api/users/${u.id}/openid-unlink`);
            showToast('OIDC link removed successfully.', 'success');
            renderUsersTab();
          } catch (err) {
            showToast('Failed to unlink OpenID: ' + err.message, 'error');
          }
        };
      }
    }

    if (canDelete) {
      tr.querySelector('.delete-user-btn').onclick = async () => {
        const confirmed = await window.showConfirm(
          'Delete User',
          `Are you sure you want to delete user "${u.username}"? This will delete all of their history and listening progress.`,
          'Delete',
          'Cancel'
        );
        if (!confirmed) {
          return;
        }
        try {
          await request('DELETE', `/api/users/${u.id}`);
          showToast('User deleted successfully.', 'success');
          renderUsersTab();
        } catch (err) {
          showToast('Failed to delete user: ' + err.message, 'error');
        }
      };
    }

    tbody.appendChild(tr);
  });
}

async function triggerUserModal(user = null, currentUser, onSaveSuccess) {
  const isEdit = !!user;
  const libraries = getLibrariesList();

  let allTags = [];
  try {
    const res = await request('GET', '/api/tags');
    allTags = res.tags || [];
  } catch (err) {
    console.error('Failed to load tags for user permissions', err);
  }

  const modal = document.createElement('div');
  modal.className = 'fixed inset-0 bg-black-900/80 z-50 flex items-center justify-center p-4';

  const perms = user?.permissions || {
    download: true,
    accessExplicitContent: false,
    accessAllLibraries: true,
    accessAllTags: true,
    selectedTagsNotAccessible: false
  };
  const libsAccessible = user?.librariesAccessible || [];
  const itemTagsSelected = user?.itemTagsSelected || [];

  const accessAllTags = perms.accessAllTags !== false;
  const selectedTagsNotAccessible = perms.selectedTagsNotAccessible === true;

  modal.innerHTML = `
    <div class="bg-primary border border-black-300 w-full max-w-lg p-6 rounded-md shadow-lg space-y-4 flex flex-col max-h-[90vh]">
      <h3 class="text-lg font-bold border-b border-black-400 pb-2 flex-shrink-0">${isEdit ? 'Edit User' : 'Add User'}</h3>
      
      <form id="user-form" class="space-y-4 flex flex-col flex-grow overflow-hidden">
        <div class="flex-grow overflow-y-auto pr-2 pb-4 space-y-6">
        <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
          <div>
            <label class="block text-xs text-black-100 mb-1">Username</label>
            <input type="text" id="user-username" required value="${isEdit ? escapeHtml(user.username) : ''}" class="w-full bg-black-500 text-white px-3 py-1.5 rounded border border-black-300 focus:outline-none focus:border-accent text-sm" ${isEdit ? 'disabled' : ''}>
          </div>
          <div>
            <label class="block text-xs text-black-100 mb-1">Email Address</label>
            <input type="email" id="user-email" value="${isEdit && user.email ? escapeHtml(user.email) : ''}" class="w-full bg-black-500 text-white px-3 py-1.5 rounded border border-black-300 focus:outline-none focus:border-accent text-sm">
          </div>
        </div>

        <div>
          <label class="block text-xs text-black-100 mb-1">Password</label>
          <input type="password" id="user-password" ${isEdit ? '' : 'required'} placeholder="${isEdit ? '•••••••• (leave blank to keep current)' : 'Enter password'}" class="w-full bg-black-500 text-white px-3 py-1.5 rounded border border-black-300 focus:outline-none focus:border-accent text-sm">
        </div>

        <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
          <div>
            <label class="block text-xs text-black-100 mb-1">Account Type</label>
            <select id="user-type" class="w-full bg-black-500 text-white px-3 py-1.5 rounded border border-black-300 focus:outline-none focus:border-accent text-sm" ${isEdit && user.type === 'root' ? 'disabled' : ''}>
              <option value="user" ${isEdit && user.type === 'user' ? 'selected' : ''}>Regular User</option>
              <option value="admin" ${isEdit && user.type === 'admin' ? 'selected' : ''}>Admin</option>
              <option value="guest" ${isEdit && user.type === 'guest' ? 'selected' : ''}>Guest</option>
              ${isEdit && user.type === 'root' ? '<option value="root" selected>Root Admin</option>' : ''}
            </select>
          </div>
          <div class="flex items-end pb-2">
            <label class="flex items-center space-x-3 cursor-pointer text-sm ${isEdit && user.type === 'root' ? 'opacity-50 pointer-events-none' : ''}">
              <span class="abs-switch">
                <input type="checkbox" id="user-isactive" ${!isEdit || user.isActive ? 'checked' : ''} ${isEdit && user.type === 'root' ? 'disabled' : ''}>
                <span class="abs-slider"></span>
              </span>
              <span>Account is Active</span>
            </label>
          </div>
        </div>

        <!-- Permissions Section -->
        <div class="space-y-4">
          <h4 class="text-xs font-semibold text-accent uppercase tracking-wider border-b border-black-400 pb-2">Permissions</h4>
          
          <div class="grid grid-cols-1 sm:grid-cols-2 gap-y-3 gap-x-6">
            <label class="flex items-center space-x-3 cursor-pointer text-sm">
              <span class="abs-switch">
                <input type="checkbox" id="perm-download" ${perms.download !== false ? 'checked' : ''}>
                <span class="abs-slider"></span>
              </span>
              <span>Allow downloading files</span>
            </label>
            <label class="flex items-center space-x-3 cursor-pointer text-sm">
              <span class="abs-switch">
                <input type="checkbox" id="perm-explicit" ${perms.accessExplicitContent ? 'checked' : ''}>
                <span class="abs-slider"></span>
              </span>
              <span>Allow explicit content access</span>
            </label>
            <label class="flex items-center space-x-3 cursor-pointer text-sm">
              <span class="abs-switch">
                <input type="checkbox" id="perm-upload" ${perms.upload === true ? 'checked' : ''}>
                <span class="abs-slider"></span>
              </span>
              <span>Allow uploading files</span>
            </label>
            <label class="flex items-center space-x-3 cursor-pointer text-sm">
              <span class="abs-switch">
                <input type="checkbox" id="perm-delete" ${perms.delete === true ? 'checked' : ''}>
                <span class="abs-slider"></span>
              </span>
              <span>Allow deleting media</span>
            </label>
            <label class="flex items-center space-x-3 cursor-pointer text-sm">
              <span class="abs-switch">
                <input type="checkbox" id="perm-update" ${perms.update === true ? 'checked' : ''}>
                <span class="abs-slider"></span>
              </span>
              <span>Allow editing metadata / library scans</span>
            </label>
            <label class="flex items-center space-x-3 cursor-pointer text-sm">
              <span class="abs-switch">
                <input type="checkbox" id="perm-rss" ${perms.accessRss === true ? 'checked' : ''}>
                <span class="abs-slider"></span>
              </span>
              <span>Allow accessing RSS feeds</span>
            </label>
            <label class="flex items-center space-x-3 cursor-pointer text-sm">
              <span class="abs-switch">
                <input type="checkbox" id="perm-shares" ${perms.createShares === true ? 'checked' : ''}>
                <span class="abs-slider"></span>
              </span>
              <span>Allow creating public shares</span>
            </label>
            <label class="flex items-center space-x-3 cursor-pointer text-sm">
              <span class="abs-switch">
                <input type="checkbox" id="perm-all-libraries" ${perms.accessAllLibraries !== false ? 'checked' : ''}>
                <span class="abs-slider"></span>
              </span>
              <span>Allow access to all libraries</span>
            </label>
          </div>

          <!-- Library Selector (Collapsible) -->
          <div id="library-selector-container" class="${perms.accessAllLibraries !== false ? 'hidden' : ''} border border-black-300 rounded p-2 bg-black-500 max-h-36 overflow-y-auto space-y-1.5">
            <p class="text-[10px] font-semibold text-black-100 uppercase mb-1">Select Libraries:</p>
            ${libraries.length === 0 ? '<p class="text-xs text-black-100">No libraries available</p>' : libraries.map(lib => `
              <label class="flex items-center space-x-2 text-xs cursor-pointer hover:bg-black-400 p-1 rounded">
                <input type="checkbox" value="${lib.id}" ${libsAccessible.includes(lib.id) ? 'checked' : ''} class="user-lib-checkbox rounded text-accent bg-black-600 border-black-300">
                <span>${escapeHtml(lib.name)}</span>
              </label>
            `).join('')}
          </div>
        </div>

        <!-- Tag Restrictions Section -->
        <div class="border-t border-black-400 pt-3 space-y-3">
          <h4 class="text-xs font-semibold text-accent uppercase tracking-wider">Tag Restrictions</h4>
          
          <div class="flex flex-col space-y-3">
            <label class="flex items-center space-x-3 cursor-pointer text-sm">
              <span class="abs-switch">
                <input type="checkbox" id="perm-all-tags" ${accessAllTags ? 'checked' : ''}>
                <span class="abs-slider"></span>
              </span>
              <span>Allow access to all tags</span>
            </label>
          </div>

          <!-- Tag Selector (Collapsible) -->
          <div id="tag-selector-container" class="${accessAllTags ? 'hidden' : ''} border border-black-300 rounded p-3 bg-black-500 space-y-3">
            <div>
              <label class="block text-[10px] font-semibold text-black-100 uppercase mb-1">Tag Filter Mode</label>
              <div class="flex rounded bg-black-600 p-1 border border-black-300 w-full" id="perm-tags-toggle-container">
                <button type="button" id="perm-tags-allow-btn" class="flex-grow py-1 text-[11px] font-semibold rounded text-center transition-all focus:outline-none ${!selectedTagsNotAccessible ? 'bg-success text-white' : 'text-black-100 hover:text-white'}" data-value="false">Allow Only Selected</button>
                <button type="button" id="perm-tags-block-btn" class="flex-grow py-1 text-[11px] font-semibold rounded text-center transition-all focus:outline-none ${selectedTagsNotAccessible ? 'bg-success text-white' : 'text-black-100 hover:text-white'}" data-value="true">Block Selected</button>
              </div>
              <input type="hidden" id="perm-tags-not-accessible" value="${selectedTagsNotAccessible ? 'true' : 'false'}">
            </div>

            <div>
              <p class="text-[10px] font-semibold text-black-100 uppercase mb-1">Select Tags:</p>
              <div class="max-h-28 overflow-y-auto space-y-1">
                ${allTags.length === 0 ? '<p class="text-[11px] text-black-100">No tags available in library</p>' : allTags.map(tag => `
                  <label class="flex items-center space-x-2 text-[11px] cursor-pointer hover:bg-black-400 p-1 rounded">
                    <input type="checkbox" value="${escapeHtml(tag)}" ${itemTagsSelected.includes(tag) ? 'checked' : ''} class="user-tag-checkbox rounded text-accent bg-black-600 border-black-300">
                    <span>${escapeHtml(tag)}</span>
                  </label>
                `).join('')}
              </div>
            </div>
          </div>
        </div>
        </div>

        <div class="flex justify-end space-x-3 pt-2 border-t border-black-400 flex-shrink-0">
          <button type="button" id="close-user-modal-btn" class="bg-black-400 hover:bg-black-300 text-white px-4 py-2 rounded text-xs font-semibold transition-colors flex items-center space-x-1">
            <span class="material-symbols text-sm">close</span>
            <span>Cancel</span>
          </button>
          <button type="submit" class="bg-accent hover:opacity-90 text-primary font-bold px-4 py-2 rounded text-xs transition-opacity flex items-center space-x-1">
            <span class="material-symbols text-sm">check</span>
            <span>${isEdit ? 'Save' : 'Create'}</span>
          </button>
        </div>
      </form>
    </div>
  `;

  document.body.appendChild(modal);
  
  const tagsNotAccessibleInput = modal.querySelector('#perm-tags-not-accessible');
  const tagsAllowBtn = modal.querySelector('#perm-tags-allow-btn');
  const tagsBlockBtn = modal.querySelector('#perm-tags-block-btn');
  if (tagsNotAccessibleInput && tagsAllowBtn && tagsBlockBtn) {
    tagsAllowBtn.onclick = () => {
      tagsNotAccessibleInput.value = 'false';
      tagsAllowBtn.className = 'flex-grow py-1 text-[11px] font-semibold rounded text-center transition-all focus:outline-none bg-success text-white';
      tagsBlockBtn.className = 'flex-grow py-1 text-[11px] font-semibold rounded text-center transition-all focus:outline-none text-black-100 hover:text-white';
    };
    tagsBlockBtn.onclick = () => {
      tagsNotAccessibleInput.value = 'true';
      tagsAllowBtn.className = 'flex-grow py-1 text-[11px] font-semibold rounded text-center transition-all focus:outline-none text-black-100 hover:text-white';
      tagsBlockBtn.className = 'flex-grow py-1 text-[11px] font-semibold rounded text-center transition-all focus:outline-none bg-success text-white';
    };
  }

  const closeModal = () => modal.remove();
  modal.querySelector('#close-user-modal-btn').onclick = closeModal;

  const allLibsCheckbox = modal.querySelector('#perm-all-libraries');
  const libContainer = modal.querySelector('#library-selector-container');
  allLibsCheckbox.onchange = () => {
    if (allLibsCheckbox.checked) {
      libContainer.classList.add('hidden');
    } else {
      libContainer.classList.remove('hidden');
    }
  };

  const allTagsCheckbox = modal.querySelector('#perm-all-tags');
  const tagContainer = modal.querySelector('#tag-selector-container');
  allTagsCheckbox.onchange = () => {
    if (allTagsCheckbox.checked) {
      tagContainer.classList.add('hidden');
    } else {
      tagContainer.classList.remove('hidden');
    }
  };

  const form = modal.querySelector('#user-form');
  form.onsubmit = async (e) => {
    e.preventDefault();

    const username = modal.querySelector('#user-username').value.trim();
    const email = modal.querySelector('#user-email').value.trim();
    const password = modal.querySelector('#user-password').value;
    const type = modal.querySelector('#user-type').value;
    const isActive = modal.querySelector('#user-isactive').checked;

    const download = modal.querySelector('#perm-download').checked;
    const accessExplicitContent = modal.querySelector('#perm-explicit').checked;
    const upload = modal.querySelector('#perm-upload').checked;
    const deleteVal = modal.querySelector('#perm-delete').checked;
    const update = modal.querySelector('#perm-update').checked;
    const accessRss = modal.querySelector('#perm-rss').checked;
    const createShares = modal.querySelector('#perm-shares').checked;
    const accessAllLibraries = allLibsCheckbox.checked;

    const libCheckboxes = modal.querySelectorAll('.user-lib-checkbox:checked');
    const librariesAccessible = Array.from(libCheckboxes).map(cb => cb.value);

    if (!accessAllLibraries && librariesAccessible.length === 0) {
      showToast('You must select at least one accessible library if "Access All Libraries" is disabled.', 'warning');
      return;
    }

    const btn = form.querySelector('button[type="submit"]');
    if (btn) {
      btn.disabled = true;
      btn.innerHTML = `<span class="animate-spin rounded-full h-3 w-3 border-b-2 border-primary inline-block mr-1"></span><span>${isEdit ? 'Saving...' : 'Creating...'}</span>`;
    }

    const accessAllTags = allTagsCheckbox.checked;
    const selectedTagsNotAccessible = modal.querySelector('#perm-tags-not-accessible').value === 'true';
    const tagCheckboxes = modal.querySelectorAll('.user-tag-checkbox:checked');
    const itemTagsSelected = Array.from(tagCheckboxes).map(cb => cb.value);

    const permissions = {
      download,
      accessExplicitContent,
      upload,
      delete: deleteVal,
      update,
      accessRss,
      createShares,
      accessAllLibraries,
      accessAllTags,
      selectedTagsNotAccessible,
      itemTagsSelected
    };

    try {
      if (isEdit) {
        const payload = {
          email,
          permissions,
          librariesAccessible,
          itemTagsSelected
        };
        if (password) {
          payload.password = password;
        }
        if (user.type !== 'root') {
          payload.type = type;
          payload.isActive = isActive;
        }
        await request('PATCH', `/api/users/${user.id}`, payload);
      } else {
        const payload = {
          username,
          password,
          email,
          type,
          isActive,
          permissions: {
            ...permissions,
            librariesAccessible,
            accessAllTags,
            itemTagsSelected,
            selectedTagsNotAccessible
          },
          librariesAccessible,
          itemTagsSelected
        };
        await request('POST', '/api/users', payload);
      }
      closeModal();
      if (onSaveSuccess) onSaveSuccess();
    } catch (err) {
      showToast('Failed to save user: ' + err.message, 'error');
      if (btn) {
        btn.disabled = false;
        btn.innerHTML = `<span class="material-symbols text-sm">check</span><span>${isEdit ? 'Save' : 'Create'}</span>`;
      }
    }
  };
}

export async function renderApiKeysTab() {
  const container = document.getElementById('tab-apikeys');
  if (!container) return;

  container.innerHTML = `<div class="animate-spin rounded-full h-8 w-8 border-b-2 border-accent mx-auto"></div>`;

  try {
    const [apiKeysResp, usersResp] = await Promise.all([
      request('GET', '/api/api-keys'),
      request('GET', '/api/users')
    ]);
    const apiKeys = apiKeysResp.apiKeys || [];
    const users = usersResp.users || [];

    container.innerHTML = `
      <div class="space-y-4">
        <div class="flex justify-between items-center">
          <h3 class="text-lg font-semibold text-white">API Keys</h3>
          <button id="add-apikey-btn" class="bg-accent hover:opacity-90 text-primary font-bold px-4 py-2 rounded shadow-sm text-xs transition-opacity flex items-center space-x-1">
            <span class="material-symbols text-sm">add</span>
            <span>Add API Key</span>
          </button>
        </div>
        
        <div class="border border-black-300 rounded-md bg-primary overflow-x-auto">
          <table class="w-full text-left text-sm">
            <thead>
              <tr class="border-b border-black-400/60 text-black-100 text-xs uppercase tracking-wider font-semibold">
                <th class="px-4 py-3">Name</th>
                <th class="px-4 py-3">User</th>
                <th class="px-4 py-3">Expires At</th>
                <th class="px-4 py-3">Created At</th>
                <th class="px-4 py-3 text-right">Actions</th>
              </tr>
            </thead>
            <tbody id="apikeys-list-rows" class="divide-y divide-black-400">
              <!-- Rows will be injected here -->
            </tbody>
          </table>
        </div>
      </div>
    `;

    const addBtn = container.querySelector('#add-apikey-btn');
    addBtn.onclick = () => {
      triggerApiKeyModal(users, () => renderApiKeysTab());
    };

    renderApiKeysListRows(apiKeys, users);
  } catch (err) {
    container.innerHTML = `<div class="text-error text-center py-4">Failed to load API keys: ${escapeHtml(err.message)}</div>`;
  }
}

function renderApiKeysListRows(apiKeys, users) {
  const tbody = document.getElementById('apikeys-list-rows');
  if (!tbody) return;

  tbody.innerHTML = '';

  if (apiKeys.length === 0) {
    tbody.innerHTML = `
      <tr>
        <td colspan="5" class="px-4 py-8 text-center text-black-100">
          No API keys generated yet.
        </td>
      </tr>
    `;
    return;
  }

  const userMap = {};
  users.forEach(u => {
    userMap[u.id] = u.username;
  });

  apiKeys.forEach(key => {
    const tr = document.createElement('tr');
    tr.className = 'hover:bg-black-500/30';

    const expiresAtFormatted = key.expiresAt ? (window.formatDateTime ? window.formatDateTime(key.expiresAt) : key.expiresAt) : 'Never';
    const createdAtFormatted = key.createdAt ? (window.formatDateTime ? window.formatDateTime(key.createdAt) : key.createdAt) : 'Unknown';
    const username = userMap[key.userId] || key.username || 'Unknown';

    tr.innerHTML = `
      <td class="px-4 py-3 font-semibold text-white">${escapeHtml(key.name || '')}</td>
      <td class="px-4 py-3 text-black-50">${escapeHtml(username)}</td>
      <td class="px-4 py-3 text-black-100">${escapeHtml(expiresAtFormatted)}</td>
      <td class="px-4 py-3 text-black-100">${escapeHtml(createdAtFormatted)}</td>
      <td class="px-4 py-3 text-right">
        <button class="delete-apikey-btn bg-red-900/40 hover:bg-red-900/60 border border-red-500/30 text-error hover:text-white hover:border-red-500/50 text-xs font-bold px-3 py-1.5 rounded shadow-sm inline-flex items-center space-x-1 transition-colors cursor-pointer" data-id="${key.id}">
          <span class="material-symbols text-sm">delete</span>
          <span>Delete</span>
        </button>
      </td>
    `;

    const deleteBtn = tr.querySelector('.delete-apikey-btn');
    deleteBtn.onclick = async () => {
      const confirmed = await window.showConfirm(
        'Delete API Key',
        `Are you sure you want to delete the API key "${key.name}"?`,
        'Delete',
        'Cancel'
      );
      if (confirmed) {
        try {
          await request('DELETE', `/api/api-keys/${key.id}`);
          showToast('API key deleted successfully.', 'success');
          renderApiKeysTab();
        } catch (err) {
          showToast('Failed to delete API key: ' + err.message, 'error');
        }
      }
    };

    tbody.appendChild(tr);
  });
}

function triggerApiKeyModal(users, onSaveSuccess) {
  const modal = document.createElement('div');
  modal.className = 'fixed inset-0 bg-black-900/80 z-50 flex items-center justify-center p-4 overflow-y-auto';

  const defaultUser = users.find(u => u.type === 'root') || users[0] || { id: '' };

  modal.innerHTML = `
    <div class="bg-primary border border-black-300 w-full max-w-md p-6 rounded-md shadow-lg space-y-4 my-8">
      <h3 class="text-lg font-bold border-b border-black-400 pb-2">Generate API Key</h3>
      
      <form id="apikey-form" class="space-y-4">
        <div>
          <label class="block text-xs text-black-100 mb-1">Key Name</label>
          <input type="text" id="apikey-name" required placeholder="e.g. Home Assistant" class="w-full bg-black-500 text-white px-3 py-1.5 rounded border border-black-300 focus:outline-none focus:border-accent text-sm">
        </div>

        <div>
          <label class="block text-xs text-black-100 mb-1">User</label>
          <select id="apikey-user" class="w-full bg-black-500 text-white px-3 py-1.5 rounded border border-black-300 focus:outline-none focus:border-accent text-sm">
            ${users.map(u => `<option value="${u.id}" ${u.id === defaultUser.id ? 'selected' : ''}>${escapeHtml(u.username)} (${u.type})</option>`).join('')}
          </select>
        </div>

        <div>
          <label class="block text-xs text-black-100 mb-1">Expires At (Optional)</label>
          <input type="datetime-local" id="apikey-expires" class="w-full bg-black-500 text-white px-3 py-1.5 rounded border border-black-300 focus:outline-none focus:border-accent text-sm">
        </div>

        <div class="flex justify-end space-x-3 pt-2">
          <button type="button" id="close-apikey-modal-btn" class="bg-black-400 hover:bg-black-300 text-white px-4 py-2 rounded text-xs font-semibold transition-colors flex items-center space-x-1">
            <span class="material-symbols text-sm">close</span>
            <span>Cancel</span>
          </button>
          <button type="submit" class="bg-accent hover:opacity-90 text-primary font-bold px-4 py-2 rounded text-xs transition-opacity flex items-center space-x-1">
            <span class="material-symbols text-sm">vpn_key</span>
            <span>Generate</span>
          </button>
        </div>
      </form>
    </div>
  `;

  document.body.appendChild(modal);

  const closeModal = () => modal.remove();
  modal.querySelector('#close-apikey-modal-btn').onclick = closeModal;

  const form = modal.querySelector('#apikey-form');
  form.onsubmit = async (e) => {
    e.preventDefault();

    const btn = form.querySelector('button[type="submit"]');
    if (btn) {
      btn.disabled = true;
      btn.innerHTML = `<span class="animate-spin rounded-full h-3 w-3 border-b-2 border-primary inline-block mr-1"></span><span>Generating...</span>`;
    }

    const name = modal.querySelector('#apikey-name').value.trim();
    const userId = modal.querySelector('#apikey-user').value;
    const expiresVal = modal.querySelector('#apikey-expires').value;

    let expiresAt = '';
    if (expiresVal) {
      expiresAt = new Date(expiresVal).toISOString();
    }

    try {
      const response = await request('POST', '/api/api-keys', {
        name,
        userId,
        expiresAt
      });
      closeModal();
      if (response && response.apiKey && response.apiKey.token) {
        triggerShowTokenModal(response.apiKey.token, onSaveSuccess);
      } else {
        if (onSaveSuccess) onSaveSuccess();
      }
    } catch (err) {
      showToast('Failed to create API key: ' + err.message, 'error');
      if (btn) {
        btn.disabled = false;
        btn.innerHTML = `<span class="material-symbols text-sm">vpn_key</span><span>Generate</span>`;
      }
    }
  };
}

function triggerShowTokenModal(token, onClosed) {
  const modal = document.createElement('div');
  modal.className = 'fixed inset-0 bg-black-900/80 z-50 flex items-center justify-center p-4 overflow-y-auto';

  modal.innerHTML = `
    <div class="bg-primary border border-black-300 w-full max-w-md p-6 rounded-md shadow-lg space-y-4 my-8">
      <h3 class="text-lg font-bold border-b border-black-400 pb-2 text-accent">API Key Generated</h3>
      
      <div class="space-y-4">
        <p class="text-sm text-black-50">
          Make sure to copy your API key now. You won't be able to see it again!
        </p>

        <div class="bg-black-500 p-3 rounded border border-black-300 flex items-center justify-between">
          <input type="text" readonly value="${escapeHtml(token)}" class="bg-transparent text-white font-mono text-xs w-full focus:outline-none select-all mr-2" id="generated-token-input">
          <button id="copy-token-btn" class="text-accent hover:text-white font-semibold text-xs shrink-0">
            Copy
          </button>
        </div>

        <div class="flex justify-end pt-2">
          <button type="button" id="close-token-modal-btn" class="bg-accent hover:opacity-90 text-primary font-bold px-4 py-2 rounded text-xs transition-opacity flex items-center space-x-1">
            <span class="material-symbols text-sm">check</span>
            <span>Done</span>
          </button>
        </div>
      </div>
    </div>
  `;

  document.body.appendChild(modal);

  const copyBtn = modal.querySelector('#copy-token-btn');
  const tokenInput = modal.querySelector('#generated-token-input');
  copyBtn.onclick = () => {
    tokenInput.select();
    try {
      document.execCommand('copy');
      copyBtn.textContent = 'Copied!';
      setTimeout(() => {
        copyBtn.textContent = 'Copy';
      }, 2000);
    } catch (err) {
      console.error('Failed to copy token: ', err);
    }
  };

  const closeModal = () => {
    modal.remove();
    if (onClosed) onClosed();
  };

  modal.querySelector('#close-token-modal-btn').onclick = closeModal;
}
