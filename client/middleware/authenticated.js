export default async function ({ store, redirect, route, app }) {
  // If the user is not authenticated
  if (!store.state.user.user) {
    if (route.name === 'batch' || route.name === 'index') {
      return redirect('/login')
    }
    return redirect(`/login?redirect=${encodeURIComponent(route.fullPath)}`)
  }

  // If user is authenticated and navigating to a library page
  if (route.params.library) {
    const libraryId = route.params.library
    // Ensure libraries list is loaded
    if (!store.state.libraries.libraries || !store.state.libraries.libraries.length) {
      await store.dispatch('libraries/load')
    }
    // Check if the navigated library ID exists in the loaded list
    const libraryExists = store.state.libraries.libraries.some((lib) => lib.id === libraryId)
    if (!libraryExists) {
      // Library not found! Let's find a fallback accessible library
      const fallbackLibrary = store.getters['libraries/getNextAccessibleLibrary']
      if (fallbackLibrary) {
        store.commit('libraries/setCurrentLibrary', { id: fallbackLibrary.id })
        return redirect(`/library/${fallbackLibrary.id}`)
      } else {
        // No libraries available at all
        return redirect('/oops?message=No libraries')
      }
    }
  }
}