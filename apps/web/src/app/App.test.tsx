import { fireEvent, render, screen, waitFor } from '@testing-library/react';

import App from './App';

describe('App', () => {
  it('renders the public home as the default entry view', () => {
    render(<App />);

    expect(screen.getByRole('heading', { name: /把验证码解析/ })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'CaptchaFlow Service Platform' })).toBeInTheDocument();
    expect(screen.queryByText(/GeeTest/i)).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: /CDK 激活 \/ 登录/ })).toBeInTheDocument();
    expect(screen.getByText('从 CDK 激活到第一次调用，只需要三步')).toBeInTheDocument();
  });

  it('enters the user console through the activation flow and changes views', async () => {
    render(<App />);

    fireEvent.click(screen.getByRole('button', { name: /CDK 激活 \/ 登录/ }));
    expect(screen.getByRole('heading', { name: '使用 CDK 进入平台' })).toBeInTheDocument();

    fireEvent.change(screen.getByLabelText('CDK 编码'), { target: { value: 'CDK-TEST' } });
    fireEvent.click(screen.getByRole('button', { name: '激活 / 登录' }));

    await waitFor(() => expect(screen.getByRole('heading', { name: '控制台概览' })).toBeInTheDocument());
    fireEvent.click(screen.getByRole('button', { name: 'API Key 管理' }));
    expect(screen.getByRole('heading', { name: 'API Key 管理' })).toBeInTheDocument();
    expect(screen.getByText('我的 API Key')).toBeInTheDocument();
  });

  it('opens the create API key dialog from the user console', async () => {
    render(<App />);

    fireEvent.click(screen.getByRole('button', { name: /CDK 激活 \/ 登录/ }));
    fireEvent.change(screen.getByLabelText('CDK 编码'), { target: { value: 'CDK-TEST' } });
    fireEvent.click(screen.getByRole('button', { name: '激活 / 登录' }));

    await waitFor(() => expect(screen.getByRole('button', { name: '新建 API Key' })).toBeInTheDocument());
    fireEvent.click(screen.getByRole('button', { name: '新建 API Key' }));
    expect(screen.getByRole('dialog', { name: '新建 API Key' })).toBeInTheDocument();
  });

  it('opens public docs and admin console from the public home', () => {
    render(<App />);

    fireEvent.click(screen.getByRole('button', { name: /查看 API 文档/ }));
    expect(screen.getByRole('heading', { name: '接入文档' })).toBeInTheDocument();
    expect(screen.getByText('CaptchaFlow API V1')).toBeInTheDocument();
    expect(screen.getByText(/\/v1\/captcha\/solve/)).toBeInTheDocument();
    expect(screen.queryByText(/GeeTest/i)).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: '登录 / 激活' }));
    expect(screen.getByRole('heading', { name: '使用 CDK 进入平台' })).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'CaptchaFlow Service Platform' }));
    fireEvent.click(screen.getByRole('button', { name: '管理员入口' }));
    expect(screen.getByRole('heading', { name: '数据概览' })).toBeInTheDocument();
    expect(screen.getByText('管理员后台')).toBeInTheDocument();
  });
});
