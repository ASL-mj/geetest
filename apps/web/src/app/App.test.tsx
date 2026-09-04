import { render, screen } from '@testing-library/react';

import App from './App';

describe('App', () => {
  it('renders the platform bootstrap shell', () => {
    render(<App />);

    expect(
      screen.getByRole('heading', { name: 'GeeTest Service Platform' }),
    ).toBeInTheDocument();
    expect(screen.getByText('Platform bootstrap ready')).toBeInTheDocument();
  });
});
