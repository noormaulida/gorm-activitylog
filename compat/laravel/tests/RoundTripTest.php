<?php

namespace Compat\Tests;

use Compat\Article;
use Spatie\Activitylog\Models\Activity;

class RoundTripTest extends TestCase
{
    public function test_schema_uses_spatie_migrations(): void
    {
        $columns = [
            'id', 'log_name', 'description', 'subject_type', 'subject_id',
            'causer_type', 'causer_id', 'properties', 'event', 'batch_uuid',
            'created_at', 'updated_at',
        ];
        foreach ($columns as $column) {
            $this->assertTrue(SchemaHas($column), "missing activity_log.{$column}");
        }
    }

    public function test_laravel_reads_go_rows(): void
    {
        if (! getenv('COMPAT_EXPECT_GO')) {
            $this->markTestSkipped('run after the Go writer');
        }

        $rows = Activity::query()->where('log_name', 'go')->orderBy('id')->get();
        $this->assertCount(5, $rows);

        $events = $rows->pluck('event')->all();
        $this->assertSame(['created', 'updated', 'deleted', 'restored', 'deleted'], $events);

        foreach ($rows as $row) {
            $this->assertSame($row->event, $row->description);
            $this->assertSame('articles', $row->subject_type);
            $this->assertNotNull($row->subject_id);
        }

        $created = $rows[0]->properties->toArray();
        $this->assertSame('Go Draft', $created['attributes']['title']);
        $this->assertArrayNotHasKey('old', $created);

        $updated = $rows[1]->properties->toArray();
        $this->assertSame('Go Draft', $updated['old']['title']);
        $this->assertSame('Go Published', $updated['attributes']['title']);

        $softDeleted = $rows[2]->properties->toArray();
        $this->assertNull($softDeleted['old']['deleted_at']);
        $this->assertNotNull($softDeleted['attributes']['deleted_at']);

        $restored = $rows[3]->properties->toArray();
        $this->assertNotNull($restored['old']['deleted_at']);
        $this->assertNull($restored['attributes']['deleted_at']);

        $hardDeleted = $rows[4]->properties->toArray();
        $this->assertArrayNotHasKey('attributes', $hardDeleted);
        $this->assertSame('Go Published', $hardDeleted['old']['title']);
    }

    public function test_laravel_writes_rows(): void
    {
        activity()->disableLogging();
        Article::withTrashed()->where('title', 'like', 'Laravel %')->forceDelete();
        Activity::query()->where('log_name', 'laravel')->delete();
        activity()->enableLogging();

        $article = Article::query()->create(['title' => 'Laravel Draft']);
        $article->update(['title' => 'Laravel Published']);
        $article->delete();
        $article->restore();
        $article->forceDelete();

        $rows = Activity::query()->where('log_name', 'laravel')->orderBy('id')->get();
        $this->assertSame(
            ['created', 'updated', 'deleted', 'restored', 'deleted'],
            $rows->pluck('event')->all(),
        );

        $softDeleted = $rows[2]->properties->toArray();
        $this->assertArrayNotHasKey('attributes', $softDeleted);
        $this->assertSame('Laravel Published', $softDeleted['old']['title']);
        $this->assertNotNull($softDeleted['old']['deleted_at']);

        $restored = $rows[3]->properties->toArray();
        $this->assertSame('Laravel Published', $restored['attributes']['title']);
        $this->assertNull($restored['attributes']['deleted_at']);
        $this->assertArrayNotHasKey('old', $restored);
    }
}

function SchemaHas(string $column): bool
{
    return \Illuminate\Support\Facades\Schema::hasColumn('activity_log', $column);
}
