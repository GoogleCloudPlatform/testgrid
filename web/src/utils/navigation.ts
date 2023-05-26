/**
 * Navigates application to a specified page.
 * @fires location-changed
 * @param {string} path
 */
export function navigate(name: string){
  const url = new URL(location.href);
  url.pathname = name;
  history.pushState(null, '', url);
  window.dispatchEvent(new CustomEvent('location-changed'));
}

/**
 * Changes the URL without reloading
 * @param {string} dashboard
 * @param {string} tab
 */
export function navigateTab(dashboard: string, tab: string){
  const url = new URL(location.href)
  if (tab === 'Summary'){
    url.pathname = `${dashboard}`
  } else {
    url.pathname = `${dashboard}/${tab}`
  }
  history.pushState(null, '', url);
  window.dispatchEvent(new CustomEvent('location-changed'));
}
