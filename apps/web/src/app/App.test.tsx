import { fireEvent, render, screen } from '@testing-library/react';

import App from './App';

describe('App', () => {
  it('renders the dashboard as the default user console view', () => {
    render(<App />);

    expect(screen.getByRole('heading', { name: '控制台概览' })).toBeInTheDocument();
    expect(screen.getByText('额度使用情况')).toBeInTheDocument();
    expect(screen.getByText('界面示例数据')).toBeInTheDocument();
  });

  it('changes views through the sidebar navigation', () => {
    render(<App />);

    fireEvent.click(screen.getByRole('button', { name: 'API Key 管理' }));

    expect(screen.getByRole('heading', { name: 'API Key 管理' })).toBeInTheDocument();
    expect(screen.getByText('我的 API Key')).toBeInTheDocument();
  });

  it('opens the create API key dialog', () => {
    render(<App />);

    fireEvent.click(screen.getByRole('button', { name: '新建 API Key' }));

    expect(screen.getByRole('dialog', { name: '新建 API Key' })).toBeInTheDocument();
  });
});
