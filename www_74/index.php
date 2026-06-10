<?php
/**
 * GopherStack Enterprise - Default Welcome Page
 * PHP <?= phpversion() ?> is running successfully!
 */
?>
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>GopherStack Enterprise - app_php74</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            font-family: 'Segoe UI', system-ui, -apple-system, sans-serif;
            background: linear-gradient(135deg, #0f0c29, #302b63, #24243e);
            color: #e0e0e0;
            min-height: 100vh;
            display: flex;
            align-items: center;
            justify-content: center;
        }
        .container {
            text-align: center;
            padding: 3rem;
            background: rgba(255,255,255,0.05);
            border-radius: 20px;
            backdrop-filter: blur(10px);
            border: 1px solid rgba(255,255,255,0.1);
            max-width: 600px;
            box-shadow: 0 25px 50px rgba(0,0,0,0.3);
        }
        h1 {
            font-size: 2.5rem;
            background: linear-gradient(135deg, #667eea, #764ba2);
            -webkit-background-clip: text;
            -webkit-text-fill-color: transparent;
            margin-bottom: 1rem;
        }
        .version {
            color: #a78bfa;
            font-size: 1.1rem;
            margin-bottom: 2rem;
        }
        .info {
            display: grid;
            grid-template-columns: 1fr 1fr;
            gap: 1rem;
            margin-top: 2rem;
        }
        .info-card {
            background: rgba(255,255,255,0.05);
            padding: 1rem;
            border-radius: 12px;
            border: 1px solid rgba(255,255,255,0.08);
        }
        .info-card h3 { color: #818cf8; font-size: 0.85rem; text-transform: uppercase; }
        .info-card p { color: #e0e0e0; font-size: 1.2rem; margin-top: 0.5rem; }
        .badge {
            display: inline-block;
            background: linear-gradient(135deg, #10b981, #059669);
            color: white;
            padding: 0.3rem 1rem;
            border-radius: 20px;
            font-size: 0.85rem;
            margin-top: 1rem;
        }
    </style>
</head>
<body>
    <div class="container">
        <h1>⚡ GopherStack - <?= htmlspecialchars("app_php74") ?></h1>
        <p class="version">High-Concurrency PHP Orchestrator for Windows Server</p>
        <span class="badge">✓ Running</span>
        <div class="info">
            <div class="info-card">
                <h3>PHP Version</h3>
                <p><?= phpversion() ?></p>
            </div>
            <div class="info-card">
                <h3>Server</h3>
                <p><?= php_sapi_name() ?></p>
            </div>
            <div class="info-card">
                <h3>OS</h3>
                <p><?= PHP_OS ?></p>
            </div>
            <div class="info-card">
                <h3>Time</h3>
                <p><?= date('H:i:s') ?></p>
            </div>
        </div>
    </div>
</body>
</html>
