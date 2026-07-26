<?php
// Basic index page - returns JSON with request info
header('Content-Type: application/json');
header('X-PHP-Version: ' . PHP_VERSION);

$response = [
    'status' => 'ok',
    'message' => 'Hello from Go-PHP Gateway',
    'php_version' => PHP_VERSION,
    'server_software' => $_SERVER['SERVER_SOFTWARE'] ?? 'unknown',
    'request_uri' => $_SERVER['REQUEST_URI'] ?? '/',
    'request_method' => $_SERVER['REQUEST_METHOD'] ?? 'GET',
    'remote_addr' => $_SERVER['REMOTE_ADDR'] ?? 'unknown',
    'timestamp' => time(),
];

echo json_encode($response, JSON_PRETTY_PRINT);
