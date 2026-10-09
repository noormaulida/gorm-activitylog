<?php

namespace Compat;

use Illuminate\Database\Eloquent\Model;
use Illuminate\Database\Eloquent\SoftDeletes;
use Spatie\Activitylog\LogOptions;
use Spatie\Activitylog\Traits\LogsActivity;

class Article extends Model
{
    use LogsActivity, SoftDeletes;

    protected $table = 'spatie_articles';

    protected $guarded = [];

    public function getActivitylogOptions(): LogOptions
    {
        return LogOptions::defaults()
            ->logOnly(['title', 'deleted_at'])
            ->logOnlyDirty()
            ->useLogName('laravel')
            ->setDescriptionForEvent(fn (string $eventName) => $eventName);
    }
}
