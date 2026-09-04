const featureChecklist = [
  'FastAPI backend health endpoint',
  'User and admin platform foundations',
  'Self-hosted PostgreSQL and Redis runtime',
];

export function App() {
  return (
    <main>
      <section>
        <p>Platform bootstrap ready</p>
        <h1>GeeTest Service Platform</h1>
        <p>
          A minimal React and Vite shell for the platform dashboard while the API, auth, and
          quota flows are implemented.
        </p>
      </section>
      <section>
        <h2>Bootstrap checklist</h2>
        <ul>
          {featureChecklist.map((item) => (
            <li key={item}>{item}</li>
          ))}
        </ul>
      </section>
    </main>
  );
}

export default App;
