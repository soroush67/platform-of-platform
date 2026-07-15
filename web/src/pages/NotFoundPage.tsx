import { Link } from "react-router-dom";

export function NotFoundPage() {
  return (
    <div className="login-shell">
      <div>
        <h1>404</h1>
        <p className="muted">Page not found.</p>
        <Link to="/orgs">Back to organizations</Link>
      </div>
    </div>
  );
}
