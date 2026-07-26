<?php
// API endpoint — demonstrates JSON API responses.
// Accepts GET and POST, returns structured JSON.

header('Content-Type: application/json');

$method = $_SERVER['REQUEST_METHOD'] ?? 'GET';
$uri    = $_SERVER['REQUEST_URI'] ?? '/';

// Route: /api/status
if ($uri === '/api/status') {
    echo json_encode([
        'status' => 'healthy',
        'uptime' => (int)php_unixtime(),
    ]);
    exit;
}

// Route: /api/echo
if ($uri === '/api/echo') {
    $input = file_get_contents('php://input');
    $data  = json_decode($input, true) ?? [];

    echo json_encode([
        'method'  => $method,
        'headers' => [
            'content_type' => $_SERVER['CONTENT_TYPE'] ?? '',
            'accept'       => $_SERVER['HTTP_ACCEPT'] ?? '',
        ],
        'body'    => $data,
        'query'   => $_GET,
    ], JSON_PRETTY_PRINT);
    exit;
}

// Route: /api/users
if ($uri === '/api/users') {
    echo json_encode([
        'users' => [
            ['id' => 1, 'name' => 'Alice', 'email' => 'alice@example.com'],
            ['id' => 2, 'name' => 'Bob',   'email' => 'bob@example.com'],
            ['id' => 3, 'name' => 'Carol', 'email' => 'carol@example.com'],
        ],
        'total' => 3,
    ], JSON_PRETTY_PRINT);
    exit;
}

// 404 for unknown routes
http_response_code(404);
echo json_encode(['error' => 'not found']);
