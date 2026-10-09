<?php

namespace Compat\Tests;

use Illuminate\Support\Facades\Schema;
use Orchestra\Testbench\TestCase as Orchestra;
use Spatie\Activitylog\ActivitylogServiceProvider;

abstract class TestCase extends Orchestra
{
    protected function getPackageProviders($app)
    {
        return [ActivitylogServiceProvider::class];
    }

    protected function defineEnvironment($app)
    {
        $app['config']->set('database.default', 'mysql');
        $app['config']->set('database.connections.mysql', [
            'driver' => 'mysql',
            'host' => getenv('DB_HOST') ?: '127.0.0.1',
            'port' => getenv('DB_PORT') ?: '3306',
            'database' => getenv('DB_DATABASE') ?: 'activitylog_spatie',
            'username' => getenv('DB_USERNAME') ?: 'root',
            'password' => getenv('DB_PASSWORD') ?: 'root',
            'charset' => 'utf8mb4',
            'collation' => 'utf8mb4_unicode_ci',
            'prefix' => '',
        ]);
    }

    protected function setUp(): void
    {
        parent::setUp();

        if (! Schema::hasTable('activity_log')) {
            $dir = dirname(__DIR__).'/vendor/spatie/laravel-activitylog/database/migrations';
            require_once $dir.'/create_activity_log_table.php.stub';
            require_once $dir.'/add_event_column_to_activity_log_table.php.stub';
            require_once $dir.'/add_batch_uuid_column_to_activity_log_table.php.stub';
            (new \CreateActivityLogTable())->up();
            (new \AddEventColumnToActivityLogTable())->up();
            (new \AddBatchUuidColumnToActivityLogTable())->up();
        }

        if (! Schema::hasTable('spatie_articles')) {
            Schema::create('spatie_articles', function ($table) {
                $table->id();
                $table->string('title');
                $table->timestamps();
                $table->softDeletes();
            });
        }
    }
}
