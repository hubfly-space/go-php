<?php
// Error handling test
header('Content-Type: application/json');

$action = $_GET['action'] ?? 'none';

switch ($action) {
    case 'warning':
        error_log("Test warning from PHP");
        $response = ['status' => 'warning triggered', 'level' => 'warning'];
        break;
    
    case 'notice':
        // This will trigger a notice if error_reporting includes notices
        $undefined_var = $undefined_var ?? 'default';
        $response = ['status' => 'notice triggered', 'value' => $undefined_var];
        break;
    
    case 'fatal':
        // Don't actually trigger fatal error in tests
        $response = ['status' => 'fatal skipped', 'reason' => 'too dangerous'];
        break;
    
    case 'headers':
        header('X-Custom-Header: test-value');
        header('X-Another-Header: another-value');
        $response = ['status' => 'headers sent'];
        break;
    
    case 'status':
        http_response_code(201);
        $response = ['status' => 'created'];
        break;
    
    case 'redirect':
        http_response_code(302);
        header('Location: /index.php');
        exit;
    
    default:
        $response = ['status' => 'ok', 'action' => $action];
        break;
}

echo json_encode($response, JSON_PRETTY_PRINT);
