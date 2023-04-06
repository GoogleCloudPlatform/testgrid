/**
 * Navigates application to a specified page.
 * @fires location-changed
 * @param {string} path
 */
export function navigate(path: string){
    const url = new URL(location.href);
    url.pathname = `dashboards`;
    history.pushState(null, '', url);
    window.dispatchEvent(new CustomEvent('location-changed'));
}
