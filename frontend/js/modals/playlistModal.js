import { request } from '../api.js';
import { escapeHtml } from '../itemDetails.js';

export function triggerAddToPlaylistModal(item, libraryId) {
  request('GET', `/api/libraries/${libraryId}/playlists`)
    .then(res => {
      const playlists = res.results || [];
      const modal = document.createElement('div');
      modal.className = 'fixed inset-0 bg-black-900/80 z-50 flex items-center justify-center p-4';
      
      const playlistOptions = playlists.map(p => `
        <option value="${p.id}">${escapeHtml(p.name)}</option>
      `).join('');
      
      modal.innerHTML = `
        <div class="bg-primary border border-black-300 w-full max-w-sm p-6 rounded-md shadow-lg space-y-4 text-left">
          <h3 class="text-lg font-bold border-b border-black-400 pb-2 text-white flex items-center space-x-1.5">
            <span class="material-symbols text-accent">playlist_add</span>
            <span>Add to Playlist</span>
          </h3>
          
          <div class="space-y-4">
            <!-- Add to Existing -->
            <div>
              <label class="block text-xs font-semibold text-black-100 uppercase tracking-wider mb-1">Choose Playlist</label>
              ${playlists.length === 0 ? `
                <p class="text-xs text-black-50 italic">No playlists created yet.</p>
              ` : `
                <select id="add-to-existing-select" class="w-full bg-black-500 text-white px-3 py-1.5 rounded border border-black-300 focus:outline-none focus:border-accent text-sm">
                  <option value="" disabled selected>-- Select a Playlist --</option>
                  ${playlistOptions}
                </select>
              `}
            </div>

            <div class="relative flex py-1 items-center">
              <div class="flex-grow border-t border-black-500"></div>
              <span class="flex-shrink mx-4 text-black-100 text-[10px] uppercase font-bold">Or</span>
              <div class="flex-grow border-t border-black-500"></div>
            </div>

            <!-- Create New -->
            <div>
              <label class="block text-xs font-semibold text-black-100 uppercase tracking-wider mb-1">New Playlist</label>
              <input type="text" id="add-to-new-playlist-name" placeholder="Playlist Name" class="w-full bg-black-500 text-white px-3 py-1.5 rounded border border-black-300 focus:outline-none focus:border-accent text-sm">
            </div>
          </div>

          <div class="flex justify-end space-x-3 pt-2">
            <button id="add-playlist-close-btn" class="bg-black-400 hover:bg-black-300 text-white px-4 py-2 rounded text-xs">Cancel</button>
            <button id="add-playlist-save-btn" class="bg-accent hover:opacity-90 text-primary font-bold px-4 py-2 rounded text-xs">Add</button>
          </div>
        </div>
      `;

      document.body.appendChild(modal);

      const closeModal = () => modal.remove();
      modal.querySelector('#add-playlist-close-btn').onclick = closeModal;

      modal.querySelector('#add-playlist-save-btn').onclick = async () => {
        const select = modal.querySelector('#add-to-existing-select');
        const playlistId = select ? select.value : '';
        const newNameInput = modal.querySelector('#add-to-new-playlist-name');
        const newName = newNameInput ? newNameInput.value.trim() : '';

        if (!playlistId && !newName) {
          alert('Please select an existing playlist or enter a name for a new one.');
          return;
        }

        try {
          if (newName) {
            // Create a new playlist with the item
            await request('POST', '/api/playlists', {
              name: newName,
              items: [item.id]
            });
            alert(`Created playlist "${newName}" and added this item.`);
          } else {
            // Add to existing playlist
            const playlist = await request('GET', `/api/playlists/${playlistId}`);
            const items = playlist.itemIds || [];
            if (items.includes(item.id)) {
              alert('Item is already in this playlist.');
              closeModal();
              return;
            }
            items.push(item.id);
            await request('PATCH', `/api/playlists/${playlistId}`, {
              items
            });
            alert('Item added to playlist successfully.');
          }
          closeModal();
        } catch (err) {
          alert('Failed to update playlist: ' + err.message);
        }
      };
    })
    .catch(err => {
      alert('Failed to load playlists: ' + err.message);
    });
}
