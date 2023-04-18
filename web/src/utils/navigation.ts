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
