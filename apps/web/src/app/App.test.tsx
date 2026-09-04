import { fireEvent, render, screen, waitFor } from '@testing-library/react';

import App from './App';

// jsdom lacks fetch-mocked activation, so stub the platform API client for
// the interaction tests; the API client itself is covered by backend
// integration tests.
const accountMock = vi.fn();

vi.mock('../lib/api', async (importOriginal) => {
  const original = await importOriginal<typeof import('../lib/api')>();
  return {
    ...original,
    api: {
      ...original.api,
      account: (...args: Parameters<typeof original.api.account>) => accountMock(...args),
      activate: vi.fn(async () => {
        throw new Error('activation unavailable in unit tests');
      }),
      logout: vi.fn(async () => ({ success: true, request_id: 'req_test' })),
      usage: vi.fn(async () => ({
        success: true,
        request_id: 'req_test',
        data: {
          quota_total: 100, quota_used: 2, quota_reserved: 0, quota_remaining: 98,
          calls_today: 2, calls_total: 2, success_total: 2, failed_total: 0,
          rejected_total: 0, success_rate: 1,
        },
      })),
      listCalls: vi.fn(async () => ({ success: true, request_id: 'req_test', data: { items: [], next_cursor: null } })),
      listKeys: vi.fn(async () => ({ success: true, request_id: 'req_test', data: { items: [] } })),
    },
  };
});

describe('App', () => {
  beforeEach(() => {
    accountMock.mockReset();
    accountMock.mockRejectedValue(new Error('session required'));
    // jsdom shares window.history across tests; every test starts from the
    // same public home route.
    window.history.replaceState({}, '', '/');
  });

  it('renders the public home as the default entry view', async () => {
    render(<App />);

    expect(await screen.findByRole('heading', { name: /把验证码解析/ })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'CaptchaFlow Service Platform' })).toBeInTheDocument();
    expect(screen.queryByText(/GeeTest/i)).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: /CDK 激活 \/ 登录/ })).toBeInTheDocument();
  });

  it('routes the activation form at /activate and reflects the URL', async () => {
    render(<App />);

    fireEvent.click(await screen.findByRole('button', { name: /CDK 激活 \/ 登录/ }));
    expect(window.location.pathname).toBe('/activate');
    expect(screen.getByRole('heading', { name: '使用 CDK 进入平台' })).toBeInTheDocument();
    expect(screen.getByLabelText('CDK 编码')).toBeInTheDocument();
    // Activation without a network mock stays on the form and surfaces an error.
    fireEvent.change(screen.getByLabelText('CDK 编码'), { target: { value: 'CAPTCHA-TEST-1234' } });
    fireEvent.click(screen.getByRole('button', { name: /激活 \/ 登录/ }));
    await waitFor(() => expect(screen.getByRole('alert')).toBeInTheDocument());
  });

  it('routes public docs at /docs from the home page', async () => {
    render(<App />);

    fireEvent.click(await screen.findByRole('button', { name: /查看 API 文档/ }));
    expect(await screen.findByRole('heading', { name: '接入文档' })).toBeInTheDocument();
    expect(window.location.pathname).toBe('/docs');
    expect(screen.getByText('CaptchaFlow API V1')).toBeInTheDocument();
    expect(screen.queryByText(/GeeTest/i)).not.toBeInTheDocument();
  });

  it('routes the admin login gate at /admin', async () => {
    render(<App />);

    fireEvent.click(await screen.findByRole('button', { name: '管理员入口' }));
    expect(window.location.pathname).toBe('/admin');
    expect(await screen.findByRole('heading', { name: '管理员登录' })).toBeInTheDocument();
    expect(screen.getByLabelText('用户名')).toBeInTheDocument();
    expect(screen.getByLabelText('密码')).toBeInTheDocument();
  });

  it('serves console deep links directly when a session cookie is valid', async () => {
    accountMock.mockResolvedValue({
      success: true,
      request_id: 'req_test',
      data: {
        cdk: {
          code_prefix: 'CAPTCHAD', status: 'ACTIVE', activated_at: null, expires_at: null,
          quota_total: 100, quota_used: 0, quota_reserved: 0, quota_remaining: 100,
        },
      },
    });
    window.history.replaceState({}, '', '/console/dashboard');
    render(<App />);

    expect(await screen.findByRole('heading', { name: '控制台概览' })).toBeInTheDocument();
    expect(await screen.findByText('剩余额度')).toBeInTheDocument();

    // Sidebar navigation updates the URL without a full reload.
    fireEvent.click(screen.getByRole('button', { name: '用量统计' }));
    expect(await screen.findByRole('heading', { name: '用量统计', level: 1 })).toBeInTheDocument();
    expect(window.location.pathname).toBe('/console/usage');
  });
});
