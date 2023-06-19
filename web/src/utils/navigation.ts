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
export function navigateTabWithoutReload(group: string, dashboard: string, tab: string){
  const url = new URL(location.href)
  url.pathname = `groups/${group}/dashboards/${dashboard}/tabs/${tab}`;
  history.pushState(null, '', url);
}

/**
 * Navigates to the dashboard URL without reload
 * @param group {string}
 * @param dashboard {string}
 */
export function navigateDashboardWithoutReload(group: string, dashboard: string){
  const url = new URL(location.href);
  url.pathname = `groups/${group}/dashboards/${dashboard}`;
  history.pushState(null, '', url);
}

/**
 * Navigates to the group URL
 * @param group {string}
 * @fires location-changed based on reload flag
 */
export function navigateGroup(group: string, reload: boolean){
  const url = new URL(location.href);
  url.pathname = `groups/${group}`;
  history.pushState(null, '', url);
  if (reload){
    window.dispatchEvent(new CustomEvent('location-changed'));
  }
}
