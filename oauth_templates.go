package main

// Default embedded OAuth templates
// These are used as fallback when external templates are not found

const defaultAuthorizePage = `<!DOCTYPE html>
<html>
<head>
    <title>Sign In</title>
    <style>
        body { font-family: Arial, sans-serif; max-width: 400px; margin: 50px auto; padding: 20px; background: #f5f5f5; }
        .login-container { background: white; padding: 30px; border-radius: 8px; box-shadow: 0 2px 10px rgba(0,0,0,0.1); }
        .header { text-align: center; margin-bottom: 30px; }
        .app-info { text-align: center; margin-bottom: 30px; color: #666; }
        .form-group { margin-bottom: 20px; }
        label { display: block; margin-bottom: 5px; font-weight: bold; }
        input[type="text"], input[type="password"] { 
            width: 100%; 
            padding: 10px; 
            border: 1px solid #ddd; 
            border-radius: 4px; 
            font-size: 16px;
            box-sizing: border-box;
        }
        input[type="text"]:focus, input[type="password"]:focus {
            border-color: #007cba;
            outline: none;
            box-shadow: 0 0 0 2px rgba(0, 124, 186, 0.2);
        }
        .btn { 
            width: 100%; 
            padding: 12px; 
            background: #007cba; 
            color: white; 
            border: none; 
            border-radius: 4px; 
            font-size: 16px; 
            cursor: pointer; 
            transition: background-color 0.2s;
        }
        .btn:hover { background: #005a87; }
        .btn:disabled { 
            background: #ccc; 
            cursor: not-allowed; 
        }
        .error { 
            color: #dc3545; 
            margin-bottom: 15px; 
            text-align: center; 
            padding: 10px;
            background-color: #f8d7da;
            border: 1px solid #f5c6cb;
            border-radius: 4px;
        }
        .loading {
            display: none;
            text-align: center;
            margin-top: 10px;
            color: #666;
        }
    </style>
    <script>
        function submitForm() {
            const btn = document.getElementById('signin-btn');
            const loading = document.getElementById('loading');
            btn.disabled = true;
            btn.textContent = 'Signing In...';
            loading.style.display = 'block';
            return true;
        }
    </script>
</head>
<body>
    <div class="login-container">
        <div class="header">
            <h2>Sign In</h2>
        </div>
        
        <div class="app-info">
            <strong>{{.ClientName}}</strong> is requesting access to <strong>{{.ResourceName}}</strong><br>
            Please sign in to continue.
        </div>
        
        {{if .ErrorMessage}}
        <div class="error">{{.ErrorMessage}}</div>
        {{end}}
        
        <form method="POST" action="/oauth/authorize" onsubmit="return submitForm()">
            <input type="hidden" name="client_id" value="{{.ClientID}}">
            <input type="hidden" name="redirect_uri" value="{{.RedirectURI}}">
            <input type="hidden" name="response_type" value="{{.ResponseType}}">
            <input type="hidden" name="scope" value="{{.Scope}}">
            <input type="hidden" name="state" value="{{.State}}">
            <input type="hidden" name="code_challenge" value="{{.CodeChallenge}}">
            <input type="hidden" name="resource" value="{{.Resource}}">
            
            <div class="form-group">
                <label for="username">Username:</label>
                <input type="text" id="username" name="username" required autocomplete="username">
            </div>
            
            <div class="form-group">
                <label for="password">Password:</label>
                <input type="password" id="password" name="password" required autocomplete="current-password">
            </div>
            
            <button type="submit" id="signin-btn" class="btn">Sign In</button>
            <div id="loading" class="loading">Authenticating...</div>
        </form>
    </div>
</body>
</html>`

const defaultSuccessPage = `<!DOCTYPE html>
<html>
<head>
    <title>Sign In Successful</title>
    <style>
        body { font-family: Arial, sans-serif; max-width: 500px; margin: 50px auto; padding: 20px; background: #f5f5f5; }
        .success-container { background: white; padding: 40px; border-radius: 8px; box-shadow: 0 2px 10px rgba(0,0,0,0.1); text-align: center; }
        .header { margin-bottom: 30px; }
        .success-icon { 
            width: 60px; 
            height: 60px; 
            border-radius: 50%; 
            background: #28a745; 
            margin: 0 auto 20px; 
            position: relative;
        }
        .success-icon::after {
            content: '';
            position: absolute;
            left: 22px;
            top: 18px;
            width: 16px;
            height: 8px;
            border: solid white;
            border-width: 0 0 3px 3px;
            transform: rotate(-45deg);
        }
        .success-message { font-size: 18px; color: #155724; margin-bottom: 30px; }
        .redirect-info { color: #666; margin-bottom: 20px; }
        .loading { margin-top: 20px; }
        .spinner { 
            border: 3px solid #f3f3f3;
            border-top: 3px solid #007cba;
            border-radius: 50%;
            width: 20px;
            height: 20px;
            animation: spin 1s linear infinite;
            margin: 0 auto;
        }
        @keyframes spin {
            0% { transform: rotate(0deg); }
            100% { transform: rotate(360deg); }
        }
        .manual-redirect { 
            margin-top: 30px; 
            padding: 15px; 
            background: #f8f9fa; 
            border-radius: 4px; 
            display: none; 
        }
        .manual-redirect a { 
            color: #007cba; 
            text-decoration: none; 
            font-weight: bold; 
        }
        .manual-redirect a:hover { 
            text-decoration: underline; 
        }
    </style>
    <script>
        let countdown = 3;
        
        function updateCountdown() {
            const countdownElement = document.getElementById('countdown');
            const redirectTextElement = document.getElementById('redirect-text');
            
            if (countdownElement) {
                countdownElement.textContent = countdown;
            }
            countdown--;
            
            if (countdown < 0) {
                if (redirectTextElement) {
                    redirectTextElement.textContent = 'Redirecting now...';
                }
                window.location.href = '{{.RedirectURL}}';
            } else {
                setTimeout(updateCountdown, 1000);
            }
        }
        
        function showManualRedirect() {
            const manualRedirectElement = document.getElementById('manual-redirect');
            if (manualRedirectElement) {
                manualRedirectElement.style.display = 'block';
            }
        }
        
        // Wait for DOM to be fully loaded
        document.addEventListener('DOMContentLoaded', function() {
            // Start countdown after DOM is ready
            updateCountdown();
            
            // Fallback for manual redirect after 10 seconds
            setTimeout(showManualRedirect, 10000);
        });
    </script>
</head>
<body>
    <div class="success-container">
        <div class="header">
            <div class="success-icon"></div>
            <h2>Sign In Successful!</h2>
        </div>
        
        <div class="success-message">
            Welcome, {{.Username}}! You have been successfully authenticated.
        </div>
        
        <div class="redirect-info">
            <span id="redirect-text">Redirecting to Claude in <span id="countdown">3</span> seconds...</span>
        </div>
        
        <div class="loading">
            <div class="spinner"></div>
        </div>
        
        <div id="manual-redirect" class="manual-redirect">
            If you are not automatically redirected, 
            <a href="{{.RedirectURL}}" target="_self">click here to continue</a>
        </div>
    </div>
</body>
</html>`