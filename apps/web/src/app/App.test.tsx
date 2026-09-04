import { fireEvent, render, screen, waitFor } from '@testing-library/react';

import App from './App';

describe('App', () => {
  it('renders the public home as the default entry view', () => {
    render(<App />);

    expect(screen.getByRole('heading', { name: /把 GeeTest 解析/ })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /CDK 激活 \/ 登录/ })).toBeInTheDocument();
    expect(screen.getByText('从注册到第一次调用，只需要三步')).toBeInTheDocument();
  });

  it('enters the user console through the activation flow and changes views', async () => {
    render(<App />);

    fireEvent.click(screen.getByRole('button', { name: /CDK 激活 \/ 登录/ }));
    expect(screen.getByRole('heading', { name: '激活你的服务' })).toBeInTheDocument();

    fireEvent.change(screen.getByLabelText('CDK 编码'), { target: { value: 'CDK-TEST' } });
    fireEvent.change(screen.getByLabelText('用户名'), { target: { value: 'tester' } });
    fireEvent.change(screen.getByLabelText('密码'), { target: { value: 'a-secure-password' } });
    fireEvent.click(screen.getByRole('button', { name: /激活并进入控制台/ }));

    await waitFor(() => expect(screen.getByRole('heading', { name: '控制台概览' })).toBeInTheDocument());
    fireEvent.click(screen.getByRole('button', { name: 'API Key 管理' }));
    expect(screen.getByRole('heading', { name: 'API Key 管理' })).toBeInTheDocument();
    expect(screen.getByText('我的 API Key')).toBeInTheDocument();
  });

  it('opens the create API key dialog from the user console', async () => {
    render(<App />);

    fireEvent.click(screen.getByRole('button', { name: /CDK 激活 \/ 登录/ }));
    fireEvent.change(screen.getByLabelText('CDK 编码'), { target: { value: 'CDK-TEST' } });
    fireEvent.change(screen.getByLabelText('用户名'), { target: { value: 'tester' } });
    fireEvent.change(screen.getByLabelText('密码'), { target: { value: 'a-secure-password' } });
    fireEvent.click(screen.getByRole('button', { name: /激活并进入控制台/ }));

    await waitFor(() => expect(screen.getByRole('button', { name: '新建 API Key' })).toBeInTheDocument());
    fireEvent.click(screen.getByRole('button', { name: '新建 API Key' }));
    expect(screen.getByRole('dialog', { name: '新建 API Key' })).toBeInTheDocument();
  });

  it('opens public docs and admin console from the public home', () => {
    render(<App />);

    fireEvent.click(screen.getByRole('button', { name: /查看 API 文档/ }));
    expect(screen.getByRole('heading', { name: '接入文档' })).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: '登录 / 激活' }));
    expect(screen.getByRole('heading', { name: '激活你的服务' })).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'GeeTest Service Platform' }));
    fireEvent.click(screen.getByRole('button', { name: '管理员入口' }));
    expect(screen.getByRole('heading', { name: '数据概览' })).toBeInTheDocument();
    expect(screen.getByText('管理员后台')).toBeInTheDocument();
  });
});
