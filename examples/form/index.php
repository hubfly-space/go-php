<?php
// Form handler — demonstrates POST form processing.
// Shows form on GET, processes on POST.

header('Content-Type: application/json');

$method = $_SERVER['REQUEST_METHOD'] ?? 'GET';

if ($method === 'GET') {
    echo json_encode([
        'form' => [
            'action'  => '/form',
            'method'  => 'POST',
            'fields'  => [
                'name'  => ['type' => 'text',     'required' => true],
                'email' => ['type' => 'email',    'required' => true],
                'age'   => ['type' => 'number',   'required' => false],
            ],
        ],
    ], JSON_PRETTY_PRINT);
    exit;
}

if ($method === 'POST') {
    $contentType = $_SERVER['CONTENT_TYPE'] ?? '';
    $body = file_get_contents('php://input');

    // Handle different content types.
    if (strpos($contentType, 'application/json') !== false) {
        $data = json_decode($body, true) ?? [];
    } elseif (strpos($contentType, 'application/x-www-form-urlencoded') !== false) {
        parse_str($body, $data);
    } elseif (strpos($contentType, 'multipart/form-data') !== false) {
        $data = $_POST;
    } else {
        $data = ['raw' => $body];
    }

    // Validate required fields.
    $errors = [];
    if (empty($data['name'])) {
        $errors[] = 'name is required';
    }
    if (empty($data['email'])) {
        $errors[] = 'email is required';
    }

    if (!empty($errors)) {
        http_response_code(422);
        echo json_encode(['errors' => $errors], JSON_PRETTY_PRINT);
        exit;
    }

    echo json_encode([
        'status' => 'created',
        'data'   => $data,
    ], JSON_PRETTY_PRINT);
    exit;
}

http_response_code(405);
echo json_encode(['error' => 'method not allowed']);
