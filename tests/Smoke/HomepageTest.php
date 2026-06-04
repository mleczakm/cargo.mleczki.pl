<?php

namespace App\Tests\Smoke;
use Symfony\Bundle\FrameworkBundle\Test\WebTestCase;

final class HomepageTest extends WebTestCase
{
    public function testNotFound(): void
    {
        $client = self::createClient();
        $client->request('GET', '/');

        $this->assertSame(404, $client->getResponse()->getStatusCode());
    }
}