import { ArrowRight, BookOpen, Check, CircleCheck, Code2, Copy, FileCode2, KeyRound, LockKeyhole, ShieldCheck, Terminal, Zap } from 'lucide-react';
import { type FormEvent, useEffect, useState } from 'react';

import { ApiError, api, getDocsBaseURL } from '../lib/api';

type PublicHeaderProps = { onBack?: () => void; onDocs: () => void; onStart: () => void; docsActive?: boolean; session?: 'checking' | 'guest' | 'authenticated'; onConsole?: () => void };
type PublicHomeProps = { onStart: () => void; onDocs: () => void; onAdmin: () => void; session?: 'checking' | 'guest' | 'authenticated'; onConsole?: () => void };
type AuthPageProps = { onBack: () => void; onDocs: () => void; onSuccess: () => void };
type PublicDocsProps = { onBack: () => void; onStart: () => void; session?: 'checking' | 'guest' | 'authenticated'; onConsole?: () => void };

function PublicHeader({ onBack, onDocs, onStart, docsActive = false, session = 'guest', onConsole }: PublicHeaderProps) {
  // With a live session the entry button becomes a console shortcut.
  const enterConsole = session === 'authenticated' && onConsole;
  return <header className="public-header"><button className="public-brand" onClick={onBack} type="button"><span className="public-brand__mark"><ShieldCheck aria-hidden="true" size={21} /></span><span>CaptchaFlow <b>Service Platform</b></span></button><nav className="public-nav" aria-label="公开导航"><button className={docsActive ? 'public-nav__item public-nav__item--active' : 'public-nav__item'} onClick={onDocs} type="button"><BookOpen aria-hidden="true" size={15} />接口文档</button><button className="public-nav__item public-nav__item--console" onClick={enterConsole ? onConsole : onStart} type="button">{enterConsole ? '控制台' : '登录 / 激活'}</button></nav></header>;
}

export function PublicHome({ onStart, onDocs, onAdmin, session = 'guest', onConsole }: PublicHomeProps) {
  return <div className="public-shell"><PublicHeader onDocs={onDocs} onStart={onStart} session={session} onConsole={onConsole} /><main className="public-home"><section className="public-hero"><div className="public-hero__copy"><div className="public-kicker"><span className="live-dot" />开发者 API 平台</div><h1>把验证码解析<br /><em>接入你的服务</em></h1><p>平台负责 CDK 身份、API Key、额度和调用审计。你的程序只需要调用一个稳定、清晰的统一接口。</p><div className="public-hero__actions"><button className="public-primary" onClick={onStart} type="button">CDK 激活 / 登录 <ArrowRight aria-hidden="true" size={17} /></button><button className="public-secondary" onClick={onDocs} type="button"><BookOpen aria-hidden="true" size={17} />查看 API 文档</button></div><div className="public-trust"><span><CircleCheck aria-hidden="true" size={15} />服务端托管凭据</span><span><CircleCheck aria-hidden="true" size={15} />幂等与额度保护</span><span><CircleCheck aria-hidden="true" size={15} />调用可追溯</span></div></div><div className="public-hero__panel"><div className="terminal-window"><div className="terminal-window__bar"><span /><span /><span /><b>request.json</b></div><pre><code>{`POST /v1/captcha/solve\n\nAuthorization: Bearer cf_live_...\nIdempotency-Key: request_01J...\n\n{\n  "captcha_id": "captcha_xxx",\n  "risk_type": "slide"\n}`}</code></pre><div className="terminal-window__response"><span className="response-dot" />200 · 1.28s · quota -1</div></div><div className="flow-note"><div><Zap aria-hidden="true" size={16} /><span>平台统一鉴权与配额</span></div><ArrowRight aria-hidden="true" size={16} /><div><ShieldCheck aria-hidden="true" size={16} /><span>底层服务安全执行</span></div></div></div></section><section className="public-section public-section--bordered"><div className="public-section__heading"><div><p className="public-eyebrow">QUICK START</p><h2>从 CDK 激活到第一次调用，只需要三步</h2></div><button className="inline-link" onClick={onDocs} type="button">阅读接入指南 <ArrowRight aria-hidden="true" size={16} /></button></div><div className="steps-grid"><article><span>01</span><KeyRound aria-hidden="true" size={20} /><h3>使用 CDK 激活</h3><p>输入管理员发放的 CDK，绑定服务身份并自动获得默认 API Key。</p></article><article><span>02</span><Code2 aria-hidden="true" size={20} /><h3>复制接入示例</h3><p>选择语言或直接使用 cURL，把平台地址加入你的服务端代码。</p></article><article><span>03</span><Terminal aria-hidden="true" size={20} /><h3>开始解析调用</h3><p>提交 Captcha ID，平台完成校验、限流、解析和结果记录。</p></article></div></section><section className="public-section"><div className="public-section__heading"><div><p className="public-eyebrow">BUILT FOR OPERATIONS</p><h2>稳定调用需要的能力，都在一个控制台</h2></div></div><div className="feature-grid"><article><span className="feature-icon feature-icon--teal"><LockKeyhole aria-hidden="true" size={20} /></span><h3>独立 API Key</h3><p>为生产、测试和不同 Worker 分别创建密钥，明文只展示一次。</p></article><article><span className="feature-icon feature-icon--blue"><Zap aria-hidden="true" size={20} /></span><h3>额度与限流</h3><p>CDK 共享额度池，预扣、退回和速率并发限制全链路可见。</p></article><article><span className="feature-icon feature-icon--orange"><FileCode2 aria-hidden="true" size={20} /></span><h3>调用可审计</h3><p>按请求 ID 查询状态、耗时、错误分类和配额流水，不记录敏感明文。</p></article></div></section></main><footer className="public-footer"><span>CaptchaFlow Service Platform · V1</span><div><button onClick={onDocs} type="button">文档</button><button onClick={onAdmin} type="button">管理员入口</button></div></footer></div>;
}

export function AuthPage({ onBack, onDocs, onSuccess }: AuthPageProps) {
  const [cdk, setCdk] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [newKey, setNewKey] = useState<string | null>(null);

  const submit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setSubmitting(true);
    setError(null);
    try {
      const result = await api.activate(cdk.trim());
      // First activation surfaces the default API key exactly once; the user
      // must copy it before entering the console.
      if (result.data?.default_api_key) {
        setNewKey(result.data.default_api_key);
        setSubmitting(false);
        return;
      }
      onSuccess();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : '网络错误，请稍后重试。');
      setSubmitting(false);
    }
  };
  if (newKey) {
    return <div className="auth-shell"><PublicHeader onBack={onBack} onDocs={onDocs} onStart={onBack} /><main className="auth-main"><section className="auth-card auth-card--wide"><div className="auth-card__heading"><h2>API Key 已创建</h2><p>默认 Key 的完整明文只显示这一次。复制并保存到服务端环境变量后再进入控制台。</p></div><div className="secret-field"><code>{newKey}</code><button className="quiet-button" onClick={() => void navigator.clipboard?.writeText(newKey)} type="button">复制完整 Key</button></div><button className="public-primary public-primary--full" onClick={onSuccess} type="button">我已安全保存，进入控制台 <ArrowRight aria-hidden="true" size={17} /></button></section></main></div>;
  }
  return <div className="auth-shell"><PublicHeader onBack={onBack} onDocs={onDocs} onStart={onBack} /><main className="auth-main"><section className="auth-aside"><div className="public-kicker"><span className="live-dot" />V1 平台入口</div><h1>一枚 CDK<br /><em>就是你的身份</em></h1><p>输入管理员发放的 CDK 即可进入控制台。平台会自动绑定服务，并在首次使用时生成默认 API Key。</p><div className="auth-aside__list"><div><Check aria-hidden="true" size={15} /><span>无需账号密码</span></div><div><Check aria-hidden="true" size={15} /><span>CDK 绑定服务额度</span></div><div><Check aria-hidden="true" size={15} /><span>同一枚 CDK 可再次进入控制台</span></div></div></section><section className="auth-card"><div className="auth-card__heading"><h2>使用 CDK 进入平台</h2><p>首次使用会创建平台身份；再次输入同一 CDK 可恢复会话。</p></div><form onSubmit={submit}><label htmlFor="auth-cdk">CDK 编码</label><input id="auth-cdk" minLength={4} onChange={(event) => setCdk(event.target.value)} placeholder="例如：CDK-XXXX-XXXX" required value={cdk} /><p className="auth-hint">CDK 是唯一的平台身份凭证，请妥善保管。</p>{error && <p className="auth-error" role="alert">{error}</p>}<button className="public-primary public-primary--full" disabled={submitting || cdk.trim().length < 4} type="submit">{submitting ? '正在进入控制台…' : '激活 / 登录'} <ArrowRight aria-hidden="true" size={17} /></button></form><div className="auth-card__footer"><ShieldCheck aria-hidden="true" size={15} /><span>平台使用短期会话 Cookie；CDK 与 API Key 不会保存在浏览器。</span></div></section></main></div>;
}

export function PublicDocs({ onBack, onStart, session = 'guest', onConsole }: PublicDocsProps) {
  // Snippets show the operator-configured display base URL when set; the
  // placeholder stays until the public meta resolves.
  const [snippet, setSnippet] = useState(`curl -X POST https://api.your-domain.com/v1/captcha/solve \\\n  -H "Authorization: Bearer cf_live_your_key" \\\n  -H "Content-Type: application/json" \\\n  -H "Idempotency-Key: your-unique-request-id" \\\n  -d '{"captcha_id":"captcha_xxx","risk_type":"slide"}'`);
  useEffect(() => {
    void getDocsBaseURL().then((base) => {
      if (base === window.location.origin) return;
      setSnippet(`curl -X POST ${base}/v1/captcha/solve \\\n  -H "Authorization: Bearer cf_live_your_key" \\\n  -H "Content-Type: application/json" \\\n  -H "Idempotency-Key: your-unique-request-id" \\\n  -d '{"captcha_id":"captcha_xxx","risk_type":"slide"}'`);
    });
  }, []);
  const copySnippet = () => void navigator.clipboard?.writeText(snippet);
  // Sidebar entries switch the visible section instead of jumping inside one
  // long page; anchors keep the existing styling with preventDefault.
  const [active, setActive] = useState<DocsSectionId>('quickstart');
  const select = (id: DocsSectionId) => { setActive(id); window.scrollTo({ top: 0 }); };
  return <div className="docs-public-shell"><PublicHeader onBack={onBack} onDocs={() => undefined} onStart={onStart} docsActive session={session} onConsole={onConsole} /><main className="docs-public-main"><aside className="docs-public-sidebar"><p className="public-eyebrow">DOCUMENTATION</p><strong>CaptchaFlow API V1</strong>{docsSections.map((section) => <a key={section.id} aria-current={active === section.id ? 'page' : undefined} className={active === section.id ? 'docs-public-sidebar__active' : undefined} href={`#${section.id}`} onClick={(event) => { event.preventDefault(); select(section.id); }}>{section.label}</a>)}<div className="docs-public-sidebar__bottom"><span>准备好开始了吗？</span><button className="inline-link" onClick={onStart} type="button">激活 CDK <ArrowRight aria-hidden="true" size={15} /></button></div></aside><div className="docs-public-content"><div className="docs-public-title"><div><p className="public-eyebrow">CAPTCHA SERVICE / API V1</p><h1>接入文档</h1><p>使用平台 API Key 调用统一的验证码解析接口。</p></div><span className="docs-version">V1 · 当前版本</span></div>{active === 'quickstart' && <article className="docs-public-article"><h2>快速开始</h2><p>平台负责用户、Key、额度、幂等和调用审计。用户程序只需要向平台地址发起 HTTPS 请求。</p><div className="docs-callout docs-callout--teal"><ShieldCheck aria-hidden="true" size={18} /><span>解析服务凭据只保存在平台后端环境中，不出现在浏览器和用户请求里。</span></div></article>}{active === 'authentication' && <article className="docs-public-article"><h2>认证方式</h2><p>在请求头中使用平台发放的 Bearer API Key，并为每次请求设置唯一的幂等键。</p><div className="docs-code"><div><span>cURL</span><button className="docs-copy" onClick={copySnippet} title="复制示例" type="button"><Copy aria-hidden="true" size={15} />复制</button></div><pre><code>{snippet}</code></pre></div></article>}{active === 'request' && <article className="docs-public-article"><h2>请求参数</h2><div className="docs-table"><div className="docs-table__row docs-table__head"><span>字段</span><span>类型</span><span>必填</span><span>说明</span></div><div className="docs-table__row"><code>captcha_id</code><span>string</span><span>是</span><span>目标验证码 ID</span></div><div className="docs-table__row"><code>risk_type</code><span>string</span><span>是</span><span>V1 固定为 <code>slide</code></span></div></div></article>}{active === 'response' && <article className="docs-public-article"><h2>响应与错误</h2><div className="docs-error-list"><div><span className="http-code http-code--success">200</span><span>解析成功，确认消耗 1 次额度。</span></div><div><span className="http-code http-code--warning">422</span><span>参数错误，不扣减额度。</span></div><div><span className="http-code http-code--danger">502</span><span>底层异常，预扣额度自动退回。</span></div></div></article>}{active === 'limits' && <article className="docs-public-article"><h2>调用限制</h2><p>请求会按用户/CDK 共享额度执行速率与并发检查。重复的 <code>Idempotency-Key</code> 返回首次请求结果，不会重复扣减。</p></article>}</div></main></div>;
}

type DocsSectionId = 'quickstart' | 'authentication' | 'request' | 'response' | 'limits';

const docsSections: Array<{ id: DocsSectionId; label: string }> = [
  { id: 'quickstart', label: '快速开始' },
  { id: 'authentication', label: '认证方式' },
  { id: 'request', label: '请求参数' },
  { id: 'response', label: '响应与错误' },
  { id: 'limits', label: '调用限制' },
];
