import '@testing-library/jest-dom';

// JSDOM does not implement layout scrolling; keep documentation navigation
// tests focused on section switching without emitting noisy console errors.
Object.defineProperty(window, 'scrollTo', {
  configurable: true,
  value: () => undefined,
});
