// <login-page> component (Users/Roles Phase 1).
//
// The real email+password sign-in screen, plus a one-time password setup
// mode (Phase 1 correction: users must be able to create their own
// password from the application, not only via a raw API call) and a link
// into the legacy Admin Key modal (Phase 1 correction: that modal's
// trigger previously lived only inside the hidden app shell, so a browser
// with neither a session nor a stored key could never reach it).
//
// Custom events dispatched (bubbles: true):
//   auth-login-success  { detail: { identity } }
//     Fired after a successful login. The shell (app.js) hides this
//     screen, shows the app shell, and updates nav/workspace state from
//     the identity in detail.
//   use-admin-key-requested
//     Fired when the user clicks "Use Admin Key instead." The shell
//     (app.js) already owns the Admin Key modal (openAdminKeyModal) and
//     its storage/validation behavior; this component does not duplicate
//     any of that, it only asks the shell to open it.

import { login, completePasswordSetup } from './auth-api-service.js';

class LoginPage extends HTMLElement {
  connectedCallback() {
    this.innerHTML = `
      <div class="login-screen d-flex align-items-center justify-content-center">
        <div class="card login-card">
          <div class="card-body p-4">
            <h4 class="mb-1">&#127921; Pool League</h4>

            <div id="login-mode">
              <p class="text-muted small mb-3">Sign in to continue</p>
              <div class="mb-2">
                <label class="form-label small mb-1" for="login-email">Email</label>
                <input type="email" class="form-control" id="login-email" autocomplete="username" required>
              </div>
              <div class="mb-3">
                <label class="form-label small mb-1" for="login-password">Password</label>
                <input type="password" class="form-control" id="login-password" autocomplete="current-password" required>
              </div>
              <div class="alert alert-danger py-2 small d-none" id="login-error"></div>
              <button type="submit" class="btn btn-primary w-100" id="login-submit-btn">Sign In</button>
              <p class="text-muted small mt-3 mb-1">
                New here? A league or system administrator sets up your account
                and gives you a one-time setup token to choose your password.
              </p>
              <div class="d-flex justify-content-between mt-2">
                <button type="button" class="btn btn-link btn-sm p-0" id="show-setup-mode-btn">Have a setup token?</button>
                <button type="button" class="btn btn-link btn-sm p-0" id="use-admin-key-btn">Use Admin Key instead</button>
              </div>
            </div>

            <div id="setup-mode" class="d-none">
              <p class="text-muted small mb-3">Choose your password</p>
              <div class="mb-2">
                <label class="form-label small mb-1" for="setup-token">Setup token</label>
                <input type="text" class="form-control" id="setup-token" autocomplete="off" required>
              </div>
              <div class="mb-2">
                <label class="form-label small mb-1" for="setup-password">New password</label>
                <input type="password" class="form-control" id="setup-password" autocomplete="new-password" required>
              </div>
              <div class="mb-3">
                <label class="form-label small mb-1" for="setup-password-confirm">Confirm password</label>
                <input type="password" class="form-control" id="setup-password-confirm" autocomplete="new-password" required>
              </div>
              <div class="alert alert-danger py-2 small d-none" id="setup-error"></div>
              <div class="alert alert-success py-2 small d-none" id="setup-success"></div>
              <button type="submit" class="btn btn-primary w-100" id="setup-submit-btn">Set Password</button>
              <button type="button" class="btn btn-link btn-sm p-0 mt-2" id="back-to-login-btn">Back to sign in</button>
            </div>
          </div>
        </div>
      </div>
    `;

    const loginMode = this.querySelector('#login-mode');
    const setupMode = this.querySelector('#setup-mode');
    const emailEl = this.querySelector('#login-email');
    const passwordEl = this.querySelector('#login-password');
    const errorEl = this.querySelector('#login-error');
    const submitBtn = this.querySelector('#login-submit-btn');

    const submit = async () => {
      errorEl.classList.add('d-none');
      const email = emailEl.value.trim();
      const password = passwordEl.value;
      if (!email || !password) {
        errorEl.textContent = 'Enter your email and password.';
        errorEl.classList.remove('d-none');
        return;
      }
      submitBtn.disabled = true;
      try {
        const identity = await login(email, password);
        passwordEl.value = '';
        this.dispatchEvent(new CustomEvent('auth-login-success', { bubbles: true, detail: { identity } }));
      } catch (e) {
        errorEl.textContent = e.message || 'Sign in failed.';
        errorEl.classList.remove('d-none');
      } finally {
        submitBtn.disabled = false;
      }
    };

    submitBtn.addEventListener('click', submit);
    loginMode.addEventListener('keydown', (e) => {
      if (e.key === 'Enter') { e.preventDefault(); submit(); }
    });

    this.querySelector('#use-admin-key-btn').addEventListener('click', () => {
      this.dispatchEvent(new CustomEvent('use-admin-key-requested', { bubbles: true }));
    });

    // --- setup mode ---------------------------------------------------

    const tokenEl = this.querySelector('#setup-token');
    const newPasswordEl = this.querySelector('#setup-password');
    const confirmEl = this.querySelector('#setup-password-confirm');
    const setupErrorEl = this.querySelector('#setup-error');
    const setupSuccessEl = this.querySelector('#setup-success');
    const setupSubmitBtn = this.querySelector('#setup-submit-btn');

    const showSetupMode = () => {
      loginMode.classList.add('d-none');
      setupMode.classList.remove('d-none');
      setupErrorEl.classList.add('d-none');
      setupSuccessEl.classList.add('d-none');
      tokenEl.focus();
    };
    const showLoginMode = () => {
      setupMode.classList.add('d-none');
      loginMode.classList.remove('d-none');
    };

    this.querySelector('#show-setup-mode-btn').addEventListener('click', showSetupMode);
    this.querySelector('#back-to-login-btn').addEventListener('click', showLoginMode);

    const submitSetup = async () => {
      setupErrorEl.classList.add('d-none');
      setupSuccessEl.classList.add('d-none');
      const token = tokenEl.value.trim();
      const newPassword = newPasswordEl.value;
      const confirm = confirmEl.value;
      if (!token || !newPassword) {
        setupErrorEl.textContent = 'Enter your setup token and a new password.';
        setupErrorEl.classList.remove('d-none');
        return;
      }
      if (newPassword.length < 8) {
        setupErrorEl.textContent = 'Password must be at least 8 characters.';
        setupErrorEl.classList.remove('d-none');
        return;
      }
      if (newPassword !== confirm) {
        setupErrorEl.textContent = 'Passwords do not match.';
        setupErrorEl.classList.remove('d-none');
        return;
      }
      setupSubmitBtn.disabled = true;
      try {
        await completePasswordSetup(token, newPassword);
        tokenEl.value = '';
        newPasswordEl.value = '';
        confirmEl.value = '';
        setupSuccessEl.textContent = 'Password set. You can now sign in.';
        setupSuccessEl.classList.remove('d-none');
        setTimeout(showLoginMode, 1500);
      } catch (e) {
        setupErrorEl.textContent = e.message || 'Could not set password -- the token may be invalid or expired.';
        setupErrorEl.classList.remove('d-none');
      } finally {
        setupSubmitBtn.disabled = false;
      }
    };

    setupSubmitBtn.addEventListener('click', submitSetup);
    setupMode.addEventListener('keydown', (e) => {
      if (e.key === 'Enter') { e.preventDefault(); submitSetup(); }
    });
  }
}

customElements.define('login-page', LoginPage);
