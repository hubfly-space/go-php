<?php
// API endpoint - handles GET and POST
header('Content-Type: application/json');

$method = $_SERVER['REQUEST_METHOD'] ?? 'GET';

switch ($method) {
    case 'GET':
        $response = [
            'method' => 'GET',
            'query' => $_GET,
            'message' => 'GET request successful',
        ];
        break;
    
    case 'POST':
        $input = json_decode(file_get_contents('php://input'), true);
        $response = [
            'method' => 'POST',
            'body' => $input,
            'message' => 'POST request successful',
        ];
        break;
    
    default:
        http_response_code(405);
        $response = [
            'error' => 'Method not allowed',
            'method' => $method,
        ];
        break;
}

echo json_encode($response, JSON_PRETTY_PRINT);
