// Minimal history-based router: URLs survive refresh and back/forward, and
// deep links work because the SPA fallback serves index.html for every path.
import { useCallback, useEffect, useState } from 'react';

export type Route =
  | { name: 'home' }
  | { name: 'docs' }
  | { name: 'activate' }
  | { name: 'admin'; section: AdminSection }
  | { name: 'console'; page: ConsolePage };

export type ConsolePage =
  | 'dashboard'
  | 'keys'
  | 'debug'
  | 'usage'
  | 'calls'
  | 'docs'
  | 'account';

export type AdminSection =
  | 'overview'
  | 'cdks'
  | 'audit'
  | 'health'
  | 'system';

const consolePages: ConsolePage[] = ['dashboard', 'keys', 'debug', 'usage', 'calls', 'docs', 'account'];
const adminSectionList: AdminSection[] = ['overview', 'cdks', 'audit', 'health', 'system'];

export function consolePath(page: ConsolePage): string {
  return `/console/${page}`;
}

export function adminPath(section: AdminSection): string {
  return `/admin/${section}`;
}

export function parsePath(pathname: string): Route {
  if (pathname === '/' || pathname === '') return { name: 'home' };
  if (pathname === '/docs') return { name: 'docs' };
  if (pathname === '/activate') return { name: 'activate' };
  if (pathname === '/admin' || pathname.startsWith('/admin/')) {
    const section = pathname.slice('/admin/'.length) as AdminSection;
    return { name: 'admin', section: adminSectionList.includes(section) ? section : 'overview' };
  }
  if (pathname.startsWith('/console/')) {
    const page = pathname.slice('/console/'.length) as ConsolePage;
    if (consolePages.includes(page)) return { name: 'console', page };
  }
  return { name: 'home' };
}

export function useRoute(): Route {
  const [route, setRoute] = useState<Route>(() => parsePath(window.location.pathname));

  useEffect(() => {
    const onPopState = () => setRoute(parsePath(window.location.pathname));
    window.addEventListener('popstate', onPopState);
    return () => window.removeEventListener('popstate', onPopState);
  }, []);

  return route;
}

// navigate pushes a new history entry and manually re-dispatches popstate so
// every subscribed useRoute() instance re-renders in the same tick.
export function navigate(path: string, replace = false): void {
  if (replace) {
    window.history.replaceState({}, '', path);
  } else {
    window.history.pushState({}, '', path);
  }
  window.dispatchEvent(new PopStateEvent('popstate'));
}

/** navigate + stable identity: components call navigate() from useNavigate(). */
export function useNavigate(): (path: string, replace?: boolean) => void {
  return useCallback((path: string, replace?: boolean) => {
    navigate(path, replace);
  }, []);
}
