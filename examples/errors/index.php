<?php
// Error handler — demonstrates PHP error output and status codes.
// Tests error scenarios: warnings, fatal errors, custom status codes.

header('Content-Type: application/json');

$uri = $_SERVER['REQUEST_URI'] ?? '/';

// Route: /error/400
if ($uri === '/error/400') {
    http_response_code(400);
    echo json_encode(['error' => 'bad request', 'message' => 'Missing required parameter']);
    exit;
}

// Route: /error/401
if ($uri === '/error/401') {
    http_response_code(401);
    header('WWW-Authenticate: Bearer realm="api"');
    echo json_encode(['error' => 'unauthorized']);
    exit;
}

// Route: /error/403
if ($uri === '/error/403') {
    http_response_code(403);
    echo json_encode(['error' => 'forbidden']);
    exit;
}

// Route: /error/404
if ($uri === '/error/404') {
    http_response_code(404);
    echo json_encode(['error' => 'not found']);
    exit;
}

// Route: /error/500
if ($uri === '/error/500') {
    http_response_code(500);
    echo json_encode(['error' => 'internal server error']);
    exit;
}

// Route: /error/warning — triggers a PHP warning
if ($uri === '/error/warning') {
    @undefined_function();
    echo json_encode(['status' => 'warning triggered']);
    exit;
}

// Route: /error/headers — tests multiple headers
if ($uri === '/error/headers') {
    header('X-Custom-1: value1');
    header('X-Custom-2: value2');
    header('X-Empty-Header:');
    header('Cache-Control: no-cache, no-store');
    echo json_encode(['status' => 'headers set']);
    exit;
}

// Default: 200
echo json_encode(['status' => 'ok']);
